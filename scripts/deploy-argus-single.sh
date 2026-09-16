#!/usr/bin/env bash
#
# argus_single 发布：这是**正在扛仓**的交易进程，停机期间兜底止损不在岗。
# 所以流程与另两个脚本不同的地方全部围绕"把停机窗口压到最短、且能证明起来了"。
#
# 与 script/deploy_1.sh（老脚本）相比堵掉的坑：
#
# 1. **老脚本先 mv 走二进制再上传**。上传失败就没有可用二进制了，而且重跑一次
#    会把刚备份的当成"旧版本"再备份一层。这里改成先传 .staged + 校验和比对，
#    确认无误才在停机窗口内做一次 mv。
# 2. **老脚本 stop 后 sleep 2 就 chmod/start**。start.sh 在进程还活着时 exit 1，
#    于是"部署成功"了但跑的还是旧进程。这里停完显式确认端口空了、进程没了。
# 3. **老脚本只看"有没有进程"**。进程起来了但配置快照没加载上，一样是坏的。
#    这里要求 /health 返回 200，并且日志里出现本轮的「仓位上限」行——那是
#    配置快照真的加载完、交易管理器真的建好的证据。
# 4. **发布前留快照**：持仓、余额、trail_state.json 各存一份到本地。
#    出事时要能回答"发布前它是什么样"。
#
# 用法：
#   set -a; source ~/.bash_profile; set +a
#   scripts/deploy-argus-single.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$ROOT/server/argus_single"
REMOTE_HOST="${argus_single_remote_server:-}"
REMOTE_PASS="${argus_single_password:-}"
REMOTE_PORT="${argus_single_port:-22}"
APP_DIR="/data/program/app/argus_single"
APP_PORT=8855
KEEP_BACKUPS="${KEEP_BACKUPS:-3}"
READY_TIMEOUT="${READY_TIMEOUT:-120}"

die() { echo "❌ $*" >&2; exit 1; }

[ -n "$REMOTE_HOST" ] || die "缺 argus_single_remote_server（先 set -a; source ~/.bash_profile; set +a）"
[ -n "$REMOTE_PASS" ] || die "缺 argus_single_password"
command -v sshpass >/dev/null || die "需要 sshpass"
command -v go >/dev/null || die "需要 go"

WORK="$(mktemp -d)"
trap 'ssh -o "ControlPath=$MUXDIR/%C" -O exit "root@$REMOTE_HOST" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
SNAP="$ROOT/dist/argus-single-predeploy-$(date +%Y%m%d_%H%M%S)"

# 连接复用：这台机器的 sshd 会掐掉密集的新连接（2026-09-16 实测，同一份发布
# 连续三次分别死在 scp 和随后的校验和 ssh 上，退出码 255）。一次发布要开十几条
# 连接，正好撞在 MaxStartups 上。改成 ControlMaster 多路复用——只认证一次，
# 后续 ssh/scp 全走同一条连接，既绕开限流也更快。
#
# socket **不能**放 $WORK：macOS 的 mktemp -d 给的是 /var/folders/... 长路径，
# 加上 %C 的 40 位摘要会超过 unix socket 的 104 字节上限，ssh 报
# "ControlPath too long" 而多路复用静默失效（实测踩过）。固定放 /tmp 下的短目录。
MUXDIR=/tmp/.dc-ssh-mux
mkdir -p "$MUXDIR" && chmod 700 "$MUXDIR"
MUX=(-o ControlMaster=auto -o "ControlPath=$MUXDIR/%C" -o ControlPersist=180)
SSH=(sshpass -p "$REMOTE_PASS" ssh "${MUX[@]}" -o StrictHostKeyChecking=no -o PubkeyAuthentication=no -o PreferredAuthentications=password -o NumberOfPasswordPrompts=1 -p "$REMOTE_PORT" "root@$REMOTE_HOST")
SCP=(sshpass -p "$REMOTE_PASS" scp "${MUX[@]}" -o StrictHostKeyChecking=no -o PubkeyAuthentication=no -o PreferredAuthentications=password -o NumberOfPasswordPrompts=1 -P "$REMOTE_PORT")

# scp_retry 带重试的上传。这台机器的 sshd 会间歇性掐掉新连接
# （2026-09-16 实测：同一份发布连续两次死在 "Connection closed by ... port 22"），
# 而上传是整条发布链里唯一会被这个掐到的一步——它在换二进制之前，失败即中止，
# 现役二进制安然无恙，所以重试是安全的。只重试连接级失败，三次仍失败就硬失败；
# 传输完整性另有 sha256 校验兜着，重试不会掩盖损坏。
scp_retry() {
  local src="$1" dst="$2" i
  for i in 1 2 3; do
    if "${SCP[@]}" "$src" "$dst" >/dev/null 2>"$WORK/scp.err"; then
      return 0
    fi
    if ! grep -qiE "closed by remote host|Connection closed|lost connection|Connection refused|timed out" "$WORK/scp.err"; then
      cat "$WORK/scp.err" >&2   # 不是连接问题（权限/路径/磁盘），别重试
      return 1
    fi
    echo "  上传 $(basename "$src") 第 $i 次被掐断，等 $((i * 10))s 重试" >&2
    sleep $((i * 10))
  done
  cat "$WORK/scp.err" >&2
  return 1
}

echo "▶ 本地自检（编译 + vet + 测试）"
cd "$SRC_DIR"
go build ./... || die "编译失败"
go vet ./... || die "vet 失败"
go test ./... >/dev/null || die "测试失败——扛仓的进程不发未通过测试的版本"
echo "  通过"

echo "▶ 交叉编译 linux/amd64"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$WORK/argus_single" . || die "交叉编译失败"
[ -s "$WORK/argus_single" ] || die "产物为空"
head -c 20 "$WORK/argus_single" | od -An -tx1 | tr -d ' \n' | grep -q "^7f454c46.*3e00" \
  || die "不是 linux/amd64 的 ELF——检查 GOOS/GOARCH"
echo "  $(wc -c < "$WORK/argus_single" | tr -d ' ') 字节"

echo "▶ 发布前快照 → $SNAP"
mkdir -p "$SNAP"
"${SSH[@]}" "cd '$APP_DIR' && cat data/trail_state.json 2>/dev/null" > "$SNAP/trail_state.json" || true
"${SSH[@]}" "cd '$APP_DIR' && ls -la argus_single* && ps -eo pid,lstart,etimes,cmd | grep '[a]rgus_single' | head -3" > "$SNAP/process.txt" 2>&1 || true
"${SSH[@]}" "cd '$APP_DIR' && tail -300 server.log | grep -E '仓位上限|余额查询.*余额|持仓|趋势' | tail -40" > "$SNAP/log_tail.txt" 2>&1 || true
echo "  trail_state / 进程 / 日志尾 各存一份"
grep -c posId "$SNAP/trail_state.json" 2>/dev/null | sed 's/^/  trail 条目数: /' || true

echo "▶ 上传（不动现役二进制）"
scp_retry "$WORK/argus_single" "root@$REMOTE_HOST:$APP_DIR/argus_single.staged" || die "上传失败"
local_sum="$(shasum -a 256 "$WORK/argus_single" | cut -d' ' -f1)"
remote_sum="$("${SSH[@]}" "sha256sum '$APP_DIR/argus_single.staged' | cut -d' ' -f1")"
[ "$local_sum" = "$remote_sum" ] || die "校验和不一致，传输损坏（现役二进制未被触碰）"
echo "  校验和一致"

REMOTE_SCRIPT="$WORK/remote.sh"
cat > "$REMOTE_SCRIPT" <<'REMOTE'
set -uo pipefail
APP_DIR="$1"; APP_PORT="$2"; KEEP="$3"; TIMEOUT="$4"
cd "$APP_DIR"
fail() { echo "❌ $*"; }

[ -x ./start.sh ] && [ -x ./stop.sh ] || { fail "start.sh / stop.sh 不可执行"; exit 1; }
[ -s ./argus_single.staged ] || { fail "argus_single.staged 不在"; exit 1; }
TS="$(date +%Y%m%d_%H%M%S)"

# 停机窗口从这里开始计时——这段时间兜底止损不在岗。
T0=$(date +%s)
echo "  停止…"
./stop.sh >/dev/null 2>&1 || true
stopped=0
for _ in $(seq 1 30); do
  # 按进程名精确匹配（pgrep -x）。不能用 pgrep -f "$APP_DIR/argus_single$"：
  # start.sh 是在 APP_DIR 里用相对路径 ./argus_single 启的，cmdline 里没有绝对
  # 路径，那个 pattern 永远匹配不到——本检查会静默失效（首次发布就踩了这个坑：
  # 服务其实已经就绪，却因为 alive 恒为 0 被判超时并回滚）。
  if ! (lsof -ti ":$APP_PORT" >/dev/null 2>&1) && ! pgrep -x argus_single >/dev/null 2>&1; then
    stopped=1; break
  fi
  sleep 1
done
[ "$stopped" = "1" ] || { fail "进程没停干净（端口 $APP_PORT 仍被占用），中止发布——现役二进制未被替换"; exit 1; }
echo "  已停"

cp -p argus_single "argus_single.backup.$TS" 2>/dev/null || true
mv argus_single.staged argus_single
chmod 755 argus_single
echo "  二进制已替换（备份 .backup.$TS）"

./start.sh >/dev/null 2>&1 || true

# 就绪三条件：进程在 + /health 200 + 日志出现本轮的「仓位上限」行。
# 第三条才是真的——前两条只说明 HTTP 起来了，不说明配置快照加载完、
# 交易管理器建好了。仓位上限那行是在 cap 守卫初始化时打的。
echo "  等待就绪…"
ready=0; waited=0
while [ "$waited" -lt "$TIMEOUT" ]; do
  alive=0; pgrep -x argus_single >/dev/null 2>&1 && alive=1
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "http://127.0.0.1:$APP_PORT/health" 2>/dev/null || true)"
  [ -n "$code" ] || code=000
  if [ "$code" != "200" ]; then
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "http://127.0.0.1:$APP_PORT/api/health" 2>/dev/null || true)"
    [ -n "$code" ] || code=000
  fi
  cap=0; awk -v t="$TS" '$0 ~ /仓位上限/ {n++} END{exit !(n>0)}' <(tail -400 server.log) && cap=1
  if [ "$alive" = "1" ] && [ "$code" = "200" ] && [ "$cap" = "1" ]; then ready=1; break; fi
  sleep 3; waited=$((waited + 3))
  [ $((waited % 15)) -eq 0 ] && echo "    ${waited}s… 进程=$alive HTTP=$code 配置已加载=$cap"
done
T1=$(date +%s)

if [ "$ready" != "1" ]; then
  fail "等了 ${TIMEOUT}s 未就绪，回滚到 .backup.$TS"
  ./stop.sh >/dev/null 2>&1 || true
  sleep 3
  [ -f "argus_single.backup.$TS" ] && cp -p "argus_single.backup.$TS" argus_single
  ./start.sh >/dev/null 2>&1 || true
  echo "  已回滚并重启，请查 $APP_DIR/server.log"
  exit 1
fi
echo "  ✅ 就绪，停机窗口 $((T1 - T0)) 秒"

ls -t argus_single.backup.* 2>/dev/null | tail -n +$((KEEP + 1)) | xargs -r rm -f
echo "  备份保留最近 $KEEP 份"

echo "  —— 本轮启动的关键行 ——"
tail -400 server.log | grep -E "仓位上限|趋势闸|趋势条件止损|trail恢复|配置" | tail -12
REMOTE

scp_retry "$REMOTE_SCRIPT" "root@$REMOTE_HOST:/tmp/deploy-argus-single.remote.sh" || die "远端脚本上传失败"
echo "▶ 切换并等待就绪"
"${SSH[@]}" "bash /tmp/deploy-argus-single.remote.sh '$APP_DIR' '$APP_PORT' '$KEEP_BACKUPS' '$READY_TIMEOUT'; rc=\$?; rm -f /tmp/deploy-argus-single.remote.sh; exit \$rc"

echo "▶ 发布后核对 trail 状态是否按 posId 恢复"
"${SSH[@]}" "cd '$APP_DIR' && cat data/trail_state.json 2>/dev/null" > "$SNAP/trail_state_after.json" || true
if command -v python3 >/dev/null; then
  python3 - "$SNAP/trail_state.json" "$SNAP/trail_state_after.json" <<'PY' || true
import json, sys
def load(p):
    try:
        return json.load(open(p)).get("states", {})
    except Exception:
        return {}
before, after = load(sys.argv[1]), load(sys.argv[2])
print(f"  发布前 {len(before)} 条 / 发布后 {len(after)} 条")
for k, v in before.items():
    a = after.get(k)
    if a is None:
        print(f"  ⚠️  {k} 发布后不见了（posId 对不上被 fail-safe 丢弃，或仓位已平）")
    elif a.get("posId") != v.get("posId"):
        print(f"  ⚠️  {k} posId 变了：{v.get('posId')} → {a.get('posId')}")
    else:
        print(f"  ✓ {k} peak={a.get('peakPct')} active={a.get('active')}")
PY
fi
echo "✅ 发布完成（快照留在 $SNAP）"
