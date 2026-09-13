#!/usr/bin/env bash
#
# 管理端前端发布：上传 → 暂存 → 切换 → 校验 → 失败自动回滚。
#
# 为什么要有这个脚本（2026-09-12 的故障）：
# 此前发布靠手贴一串 `mv A B && mv C A && pm2 restart`。那串命令**不防重复执行**：
# 第一个 mv 无条件把线上目录挪走，若暂存目录已被上一次消费掉，第二个 mv 就失败，
# set -e 当场中止，于是 /data/program/app/manager-fe **不存在**了。
# 而 pm2 里的进程抓着已被改名的目录句柄继续跑，`pm2 list` 是 online、
# 首页也还是 200（走 x-nextjs-cache），**只有 /_next/static/* 静默 500**——
# 页面在裸 antd 样式下渲染，看着只是"有点丑"。这个状态持续了约 5 小时没被发现。
#
# 所以这里有三条硬规矩：
#   1. 切换前校验：线上目录在、暂存目录在且完整（server.js + .next/BUILD_ID）；
#   2. 校验必须**同时**查 HTML 与一个静态资源——只查 HTML 正是上次漏掉故障的原因；
#   3. 校验不过就自动回滚，不留半吊子状态。
#
# 用法：
#   set -a; source ~/.bash_profile; set +a     # 取 argus_single_remote_server / _password
#   scripts/deploy-manager-fe.sh /path/to/manager-fe-vN.tar.gz
#
# 包的内容要求：解开后就是 Next standalone 的根（server.js / .next/ / node_modules/）。
set -euo pipefail

TARBALL="${1:-}"
REMOTE_HOST="${argus_single_remote_server:-}"
REMOTE_PASS="${argus_single_password:-}"
REMOTE_PORT="${argus_single_port:-22}"
APP_DIR="/data/program/app"
APP="manager-fe"
KEEP_BACKUPS="${KEEP_BACKUPS:-3}"
# 校验超时。Next standalone 起得快，但机器繁忙时给足余量。
READY_TIMEOUT="${READY_TIMEOUT:-60}"

die() { echo "❌ $*" >&2; exit 1; }

[ -n "$TARBALL" ] || die "用法: $0 <manager-fe-vN.tar.gz>"
[ -f "$TARBALL" ] || die "找不到包: $TARBALL"
[ -n "$REMOTE_HOST" ] || die "缺 argus_single_remote_server（先 set -a; source ~/.bash_profile; set +a）"
[ -n "$REMOTE_PASS" ] || die "缺 argus_single_password"
command -v sshpass >/dev/null || die "需要 sshpass"

# 本地先验包：解开后必须是 standalone 根。包错了不该等到线上才发现。
tar tzf "$TARBALL" >/dev/null 2>&1 || die "不是合法的 tar.gz: $TARBALL"
for entry in "./server.js" "./.next/BUILD_ID"; do
  tar tzf "$TARBALL" | grep -qx "$entry" || die "包里缺 $entry —— 这不像 Next standalone 产物"
done

SSH=(sshpass -p "$REMOTE_PASS" ssh -o StrictHostKeyChecking=no -o PubkeyAuthentication=no -o PreferredAuthentications=password -o NumberOfPasswordPrompts=1 -p "$REMOTE_PORT" "root@$REMOTE_HOST")
SCP=(sshpass -p "$REMOTE_PASS" scp -o StrictHostKeyChecking=no -o PubkeyAuthentication=no -o PreferredAuthentications=password -o NumberOfPasswordPrompts=1 -P "$REMOTE_PORT")

BASENAME="$(basename "$TARBALL")"
echo "▶ 上传 $BASENAME"
"${SCP[@]}" "$TARBALL" "root@$REMOTE_HOST:$APP_DIR/$BASENAME" >/dev/null

LOCAL_SUM="$(shasum -a 256 "$TARBALL" | cut -d' ' -f1)"
REMOTE_SUM="$("${SSH[@]}" "sha256sum '$APP_DIR/$BASENAME' | cut -d' ' -f1")"
[ "$LOCAL_SUM" = "$REMOTE_SUM" ] || die "校验和不一致，传输损坏"
echo "  校验和一致"

# 远端动作写成脚本传过去执行：嵌在 ssh 单引号里做多层转义，正是上次出错的土壤。
REMOTE_SCRIPT="$(mktemp)"
trap 'rm -f "$REMOTE_SCRIPT"' EXIT
cat > "$REMOTE_SCRIPT" <<'REMOTE'
set -euo pipefail
APP_DIR="$1"; APP="$2"; BASENAME="$3"; KEEP="$4"; TIMEOUT="$5"
cd "$APP_DIR"
export PATH="$PATH:$APP_DIR/node/bin"

LIVE="$APP"
TS="$(date +%Y%m%d_%H%M%S)"
STAGE="$APP.new.$TS"
BACKUP="$APP.backup.$TS"

# ---- 切换前置校验 ----------------------------------------------------------
[ -d "$LIVE" ] || { echo "❌ 线上目录 $APP_DIR/$LIVE 不存在——先人工确认当前状态，不要盲目发布"; exit 1; }
[ -f "$LIVE/pm2.config.js" ] || { echo "❌ 线上目录里没有 pm2.config.js"; exit 1; }

mkdir -p "$STAGE"
tar xzf "$BASENAME" -C "$STAGE"
cp "$LIVE/pm2.config.js" "$STAGE/pm2.config.js"

[ -f "$STAGE/server.js" ] && [ -s "$STAGE/.next/BUILD_ID" ] || { echo "❌ 暂存目录不完整"; rm -rf "$STAGE"; exit 1; }
NEW_BUILD="$(cat "$STAGE/.next/BUILD_ID")"
OLD_BUILD="$(cat "$LIVE/.next/BUILD_ID" 2>/dev/null || echo '-')"
echo "  $OLD_BUILD -> $NEW_BUILD"

# 挑一个用于事后校验的静态资源。**必须在并入旧文件之前挑**，否则可能挑中
# 上一版并进来的文件——那种文件就算新产物有问题也照样能返回 200，校验就白做了。
# 另外：**只查首页是查不出这类故障的**，首页走 x-nextjs-cache 照样 200，
# 而 /_next/static/* 可以全 500（2026-09-12 就是这么漏过去 5 小时的）。
ASSET="$(ls "$STAGE/.next/static/css/"*.css 2>/dev/null | head -1 || true)"
[ -n "$ASSET" ] || ASSET="$(ls "$STAGE/.next/static/chunks/"*.js 2>/dev/null | head -1 || true)"
[ -n "$ASSET" ] || { echo "❌ 新产物里找不到任何静态资源，无法校验"; rm -rf "$STAGE"; exit 1; }
ASSET_URL="/_next/static/${ASSET#*/.next/static/}"

# 上一版的静态资源并进来：chunk 名是内容哈希不会冲突，换版前打开的标签页
# 仍能取到它记着的那个文件，不会 404。-n 保证新文件不被旧的覆盖。
cp -rn "$LIVE/.next/static/." "$STAGE/.next/static/" 2>/dev/null || true

# ---- 切换 ------------------------------------------------------------------
mv "$LIVE" "$BACKUP"
mv "$STAGE" "$LIVE"
cd "$LIVE"
pm2 restart pm2.config.js --update-env >/dev/null 2>&1 || true

# ---- 校验：HTML 与静态资源都要 200 ----------------------------------------
ok=0
for _ in $(seq 1 "$TIMEOUT"); do
  html="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://127.0.0.1:9701/ || echo 000)"
  asset="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:9701$ASSET_URL" || echo 000)"
  if [ "$html" = "200" ] && [ "$asset" = "200" ]; then ok=1; break; fi
  sleep 1
done

if [ "$ok" != "1" ]; then
  echo "❌ 校验失败（HTML=$html 静态资源=$asset），回滚到 $BACKUP"
  cd "$APP_DIR"
  rm -rf "$LIVE"
  mv "$BACKUP" "$LIVE"
  cd "$LIVE" && pm2 restart pm2.config.js --update-env >/dev/null 2>&1 || true
  exit 1
fi

echo "  ✅ HTML 200 / 静态资源 200 ($ASSET_URL)"
cd "$APP_DIR"
rm -f "$BASENAME"
# 旧备份只留最近几份，别把盘吃满。
ls -dt "$APP.backup."* 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -rf
echo "  备份保留: $(ls -dt "$APP.backup."* 2>/dev/null | tr '\n' ' ')"
REMOTE

"${SCP[@]}" "$REMOTE_SCRIPT" "root@$REMOTE_HOST:/tmp/deploy-manager-fe.remote.sh" >/dev/null
echo "▶ 切换并校验"
"${SSH[@]}" "bash /tmp/deploy-manager-fe.remote.sh '$APP_DIR' '$APP' '$BASENAME' '$KEEP_BACKUPS' '$READY_TIMEOUT'; rc=\$?; rm -f /tmp/deploy-manager-fe.remote.sh; exit \$rc"
echo "✅ 发布完成"
