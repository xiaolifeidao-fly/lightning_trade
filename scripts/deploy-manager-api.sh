#!/usr/bin/env bash
#
# manager-api 发布：交叉编译 → 上传 → 换二进制 → 重启 → 等就绪 → 失败回滚。
#
# 为什么要脚本（都是手贴命令踩过的）：
#
# 1. **启动要两分钟**。日志实测 "Router initialized successfully in 2m0.5s"——
#    每个 handler 启动时都跑 AutoMigrate（trade 48 秒、strategy 31 秒）。
#    这期间端口根本没监听，整个控制台 502。此前给的命令是 `sleep 3` 之后 curl 一次，
#    那一次必然失败或误判，等于没校验。这里改成**轮询到真的就绪**，并打印用时。
#
# 2. **start.sh 在进程没停干净时会 exit 0**（"already running"）。
#    于是"发布成功"了，实际还在跑旧二进制，而且毫无迹象。这里停完显式确认
#    端口空了、进程没了，才允许启动。
#
# 3. **CLI 与服务端共用 service 层**，必须一起发。argus-session-rotate 曾经
#    落后于 manager-api 好几个提交，守卫改了它却还是旧的。
#
# 不碰 configs/：线上配置只在服务器上，发布只换二进制。
#
# 用法：
#   set -a; source ~/.bash_profile; set +a
#   scripts/deploy-manager-api.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT/server/manager-api"
REMOTE_HOST="${argus_single_remote_server:-}"
REMOTE_PASS="${argus_single_password:-}"
REMOTE_PORT="${argus_single_port:-22}"
APP_DIR="/data/program/app/manager-api"
KEEP_BACKUPS="${KEEP_BACKUPS:-3}"
# 就绪超时。启动实测约 120 秒，留一倍余量；机器繁忙时可用环境变量调大。
READY_TIMEOUT="${READY_TIMEOUT:-300}"

die() { echo "❌ $*" >&2; exit 1; }

[ -n "$REMOTE_HOST" ] || die "缺 argus_single_remote_server（先 set -a; source ~/.bash_profile; set +a）"
[ -n "$REMOTE_PASS" ] || die "缺 argus_single_password"
command -v sshpass >/dev/null || die "需要 sshpass"
command -v go >/dev/null || die "需要 go"

# 一起发的产物：服务端 + 两个共用 service 层的 CLI。
BINARIES=(manager-api argus-session-rotate argus-episode-rebuild)
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "▶ 交叉编译 linux/amd64"
cd "$SRC_DIR"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$WORK/manager-api" cmd.go || die "manager-api 编译失败"
for cli in argus-session-rotate argus-episode-rebuild; do
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$WORK/$cli" "./cmd/$cli" || die "$cli 编译失败"
done

# 产物自检：必须是 amd64 的 ELF。交叉编译变量写错时产出的是 macOS 可执行文件，
# 传上去只会得到一句 "cannot execute binary file"，不如在本地就拦住。
for b in "${BINARIES[@]}"; do
  [ -s "$WORK/$b" ] || die "$b 产物为空"
  head -c 20 "$WORK/$b" | od -An -tx1 | tr -d ' \n' | grep -q "^7f454c46.*3e00" \
    || die "$b 不是 linux/amd64 的 ELF——检查 GOOS/GOARCH"
  printf "  %-22s %s\n" "$b" "$(wc -c < "$WORK/$b" | tr -d ' ') 字节"
done

SSH=(sshpass -p "$REMOTE_PASS" ssh -o StrictHostKeyChecking=no -o PubkeyAuthentication=no -o PreferredAuthentications=password -o NumberOfPasswordPrompts=1 -p "$REMOTE_PORT" "root@$REMOTE_HOST")
SCP=(sshpass -p "$REMOTE_PASS" scp -o StrictHostKeyChecking=no -o PubkeyAuthentication=no -o PreferredAuthentications=password -o NumberOfPasswordPrompts=1 -P "$REMOTE_PORT")

echo "▶ 上传"
for b in "${BINARIES[@]}"; do
  "${SCP[@]}" "$WORK/$b" "root@$REMOTE_HOST:$APP_DIR/$b.staged" >/dev/null
  local_sum="$(shasum -a 256 "$WORK/$b" | cut -d' ' -f1)"
  remote_sum="$("${SSH[@]}" "sha256sum '$APP_DIR/$b.staged' | cut -d' ' -f1")"
  [ "$local_sum" = "$remote_sum" ] || die "$b 校验和不一致，传输损坏"
done
echo "  校验和全部一致"

REMOTE_SCRIPT="$WORK/remote.sh"
cat > "$REMOTE_SCRIPT" <<'REMOTE'
set -uo pipefail
APP_DIR="$1"; KEEP="$2"; TIMEOUT="$3"; shift 3
BINARIES=("$@")
PORT=8491
cd "$APP_DIR"

fail() { echo "❌ $*"; }

# ---- 前置校验 --------------------------------------------------------------
[ -x "./start.sh" ] && [ -x "./stop.sh" ] || { fail "start.sh / stop.sh 不可执行"; exit 1; }
[ -f "./configs/application.properties" ] || { fail "configs/application.properties 不在——发布只换二进制，配置必须已就位"; exit 1; }
for b in "${BINARIES[@]}"; do
  [ -s "$b.staged" ] || { fail "$b.staged 不在"; exit 1; }
done

TS="$(date +%Y%m%d_%H%M%S)"

# ---- 停 --------------------------------------------------------------------
echo "  停止 manager-api…"
./stop.sh >/dev/null 2>&1 || true
# start.sh 在"还在跑"时会直接 exit 0，于是换了二进制也还是旧进程在服务。
# 所以这里必须确认真的停了，停不掉就中止，绝不带着旧进程继续。
stopped=0
for _ in $(seq 1 20); do
  if ! (lsof -ti ":$PORT" >/dev/null 2>&1) && ! pgrep -f "$APP_DIR/manager-api$" >/dev/null 2>&1; then
    stopped=1; break
  fi
  sleep 1
done
[ "$stopped" = "1" ] || { fail "进程没停干净（端口 $PORT 仍被占用），中止发布"; exit 1; }
echo "  已停"

# ---- 换二进制 --------------------------------------------------------------
for b in "${BINARIES[@]}"; do
  [ -f "$b" ] && cp -p "$b" "$b.backup.$TS"
  mv "$b.staged" "$b"
  chmod 755 "$b"
done
echo "  二进制已替换（备份后缀 .backup.$TS）"

# ---- 起 --------------------------------------------------------------------
./start.sh >/dev/null 2>&1 || true

# ---- 等就绪 ----------------------------------------------------------------
# 两段判定：先等端口开始应答（路由注册完才会监听），再要求一个已知路由返回 200
# （证明 handler 注册了、库也连得上）。启动期间整个控制台 502，所以这一步
# 必须真的等到，不能 sleep 几秒就宣布成功。
echo "  等待就绪（启动期约 2 分钟，每个 handler 都要跑 AutoMigrate）…"
ready=0; waited=0
while [ "$waited" -lt "$TIMEOUT" ]; do
  # curl 连不上时 -w 已经会输出 000 并且退出码非 0，再 `|| echo 000` 就成了 "000000"。
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://127.0.0.1:$PORT/api/argus-config/instances" 2>/dev/null || true)"
  [ -n "$code" ] || code=000
  if [ "$code" = "200" ]; then ready=1; break; fi
  sleep 5; waited=$((waited + 5))
  [ $((waited % 30)) -eq 0 ] && echo "    ${waited}s… (当前 HTTP $code)"
done

if [ "$ready" != "1" ]; then
  fail "等了 ${TIMEOUT}s 仍未就绪，回滚到 .backup.$TS"
  ./stop.sh >/dev/null 2>&1 || true
  sleep 3
  for b in "${BINARIES[@]}"; do
    [ -f "$b.backup.$TS" ] && mv "$b.backup.$TS" "$b"
  done
  ./start.sh >/dev/null 2>&1 || true
  echo "  已回滚并重启，请查 $APP_DIR/logs/manager-api.log"
  exit 1
fi

echo "  ✅ 就绪，用时 ${waited}s"
for b in "${BINARIES[@]}"; do
  ls -t "$b.backup."* 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -f
done
echo "  备份保留每个二进制最近 $KEEP 份"
REMOTE

"${SCP[@]}" "$REMOTE_SCRIPT" "root@$REMOTE_HOST:/tmp/deploy-manager-api.remote.sh" >/dev/null
echo "▶ 切换并等待就绪"
"${SSH[@]}" "bash /tmp/deploy-manager-api.remote.sh '$APP_DIR' '$KEEP_BACKUPS' '$READY_TIMEOUT' ${BINARIES[*]}; rc=\$?; rm -f /tmp/deploy-manager-api.remote.sh; exit \$rc"
echo "✅ 发布完成"
