#!/usr/bin/env bash
#
# 每周凭证轮换：浏览器抓的 curl → session.json → 服务器上的 argus-session-rotate。
#
# 背景：账户是 login_type=config（静态凭证），trade.BuildUserProvider 走
# StaticUserProvider，**进程永远不会自己重登**——只有 password 模式才会去调
# pl-instance 的无头登录，而生产上没部署它。所以 cookie 一到期，下单就直接死，
# 而且**余额查询仍然正常**（余额/持仓走的是 apiKey HMAC 的原生通道，见
# close_channel.go），日志上看一切岁月静好。唯一的症状是下单返回
#   code=1000010 msg="GW: Login Timeout"
#
# 用法：
#   scripts/rotate-argus-cookies.sh                 # dry run，只看计划
#   scripts/rotate-argus-cookies.sh --apply         # 真写库 + 通知实例热加载
#   scripts/rotate-argus-cookies.sh --instance argus-single-fly --apply
#
# 输入文件默认 server/argus_single/configs/cookies，内容就是浏览器 DevTools
# 里对任意一条带登录态的请求「Copy as cURL」，**每个账号一块，直接往下贴**。
# 该目录整个在 .gitignore 里（server/argus_single/.gitignore: configs/*）。
#
# 全程不打印任何明文凭证，只打印 uid 与长度。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COOKIES="${COOKIES:-$ROOT/server/argus_single/configs/cookies}"
INSTANCE="argus-single-roc"
APPLY=0
APP_DIR="/data/program/app/manager-api"

while [ $# -gt 0 ]; do
  case "$1" in
    --instance) INSTANCE="$2"; shift 2 ;;
    --file)     COOKIES="$2"; shift 2 ;;
    --apply)    APPLY=1; shift ;;
    -h|--help)  sed -n '2,28p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "未知参数 $1" >&2; exit 1 ;;
  esac
done

die() { echo "❌ $*" >&2; exit 1; }

REMOTE_HOST="${argus_single_remote_server:-}"
REMOTE_PASS="${argus_single_password:-}"
REMOTE_PORT="${argus_single_port:-22}"
[ -n "$REMOTE_HOST" ] || die "缺 argus_single_remote_server（先 set -a; source ~/.bash_profile; set +a）"
[ -n "$REMOTE_PASS" ] || die "缺 argus_single_password"
[ -f "$COOKIES" ] || die "找不到 $COOKIES"
command -v sshpass >/dev/null || die "需要 sshpass"

WORK="$(mktemp -d)"
# 明文凭证落地的这一小段时间里，至少不让同机其他用户读到。
chmod 700 "$WORK"
trap 'rm -rf "$WORK"' EXIT

echo "▶ 解析 $COOKIES"
python3 - "$COOKIES" "$WORK/session.json" <<'PY'
import json, re, sys, datetime

src, dst = sys.argv[1], sys.argv[2]
text = open(src, encoding="utf-8").read()

def unquote(s):
    return s.replace("'\\''", "'")   # Chrome 把单引号转义成 '\''

# 分块：第二块往往带前导空格（粘贴时缩进），所以不能用 startswith("curl ")。
# 这个细节真的咬过人——不 lstrip 的话两块会被当成一块，headers 后者覆盖前者，
# 结果是把 A 的 cookie 配上 B 的 uid 发出去。下面还有一道 uid 唯一性兜底。
blocks, cur = [], []
for line in text.splitlines():
    if line.lstrip().startswith("curl "):
        if cur:
            blocks.append("\n".join(cur))
        cur = [line]
    elif cur:
        cur.append(line)
if cur:
    blocks.append("\n".join(cur))
if not blocks:
    sys.exit("文件里没有 curl 块——确认粘贴的是 DevTools 的 Copy as cURL")

QUOTED = r"'((?:[^']|'\\'')*)'"
out, seen = {}, set()
for block in blocks:
    joined = block.replace("\\\n", " ")
    headers = {}
    for m in re.finditer(r"-H\s+" + QUOTED, joined):
        raw = unquote(m.group(1))
        if ":" in raw:
            k, v = raw.split(":", 1)
            headers[k.strip().lower()] = v.strip()
    uids = {unquote(m.group(1)).split(":", 1)[1].strip()
            for m in re.finditer(r"-H\s+" + QUOTED, joined)
            if unquote(m.group(1)).lower().startswith("uid:")}
    if len(uids) > 1:
        sys.exit("一个 curl 块里出现了 %d 个 uid %s——说明分块没分开，"
                 "继续会把一个账号的凭证写到另一个头上" % (len(uids), sorted(uids)))

    mb = re.search(r"-b\s+" + QUOTED, joined)
    cookie = unquote(mb.group(1)) if mb else headers.get("cookie", "")
    uid, token = headers.get("uid", ""), headers.get("token", "")
    if not uid:
        sys.exit("有一块缺 uid 头，无法认领到账户")
    if not cookie or not token:
        sys.exit("uid %s 缺 cookie 或 token（cookie=%d字 token=%d字）" % (uid, len(cookie), len(token)))
    if uid in seen:
        sys.exit("uid %s 出现了两次，确认是不是同一个账号贴了两遍" % uid)
    seen.add(uid)

    ts = headers.get("timestamp", "")
    when = (datetime.datetime.fromtimestamp(int(ts) / (1000 if len(ts) > 10 else 1)).astimezone()
            if ts.isdigit() else datetime.datetime.now().astimezone())
    # 只写会轮换的三件套。baggage / sentry* 故意留空：库里现在就是空的且下单正常，
    # 而 baggage 里含一个固定的 sentry trace id，常年发同一个反倒成了指纹。
    # 服务端按 uid 认领账户（matchIncoming 里 uid 优先于名字），所以键名用 uid 就够。
    out[uid] = {"uid": uid, "cookie": cookie, "token": token,
                "otoken": headers.get("otoken", ""),
                "updatedAt": when.isoformat(timespec="seconds")}

with open(dst, "w", encoding="utf-8") as f:
    json.dump({"accounts": out}, f, ensure_ascii=False, indent=2)

print("  解析出 %d 个账号：" % len(out))
for uid, v in out.items():
    print("    uid=%-9s cookie=%d字 token=%d字 otoken=%d字 抓于 %s"
          % (uid, len(v["cookie"]), len(v["token"]), len(v["otoken"]), v["updatedAt"]))
PY
chmod 600 "$WORK/session.json"

CM="/tmp/.argus-rotate-cm-$$.sock"
SSHOPT=(-o StrictHostKeyChecking=no -o PubkeyAuthentication=no -o PreferredAuthentications=password
        -o NumberOfPasswordPrompts=1 -o ControlMaster=auto -o ControlPath="$CM" -o ControlPersist=120)
SSH=(sshpass -p "$REMOTE_PASS" ssh "${SSHOPT[@]}" -p "$REMOTE_PORT" "root@$REMOTE_HOST")
SCP=(sshpass -p "$REMOTE_PASS" scp "${SSHOPT[@]}" -P "$REMOTE_PORT")
# 明文只在服务器上存在这一小会儿，不管成功失败都擦掉。
cleanup_remote() { "${SSH[@]}" 'shred -u /tmp/argus-rotate/session.json 2>/dev/null || rm -f /tmp/argus-rotate/session.json; rmdir /tmp/argus-rotate 2>/dev/null' >/dev/null 2>&1 || true; }
trap 'cleanup_remote; rm -rf "$WORK"' EXIT

echo "▶ 上传到 $REMOTE_HOST"
"${SSH[@]}" 'install -d -m 700 /tmp/argus-rotate'
# 文件名必须正好是 session.json：LoadRotateSessionFile 会校验 basename。
"${SCP[@]}" "$WORK/session.json" "root@$REMOTE_HOST:/tmp/argus-rotate/session.json" >/dev/null
local_sum="$(shasum -a 256 "$WORK/session.json" | cut -d' ' -f1)"
remote_sum="$("${SSH[@]}" 'chmod 600 /tmp/argus-rotate/session.json; sha256sum /tmp/argus-rotate/session.json | cut -d" " -f1')"
[ "$local_sum" = "$remote_sum" ] || die "校验和不一致，传输损坏"
echo "  校验和一致"

if [ "$APPLY" = "0" ]; then
  echo "▶ DRY RUN（实例 $INSTANCE）"
  "${SSH[@]}" "cd $APP_DIR && ./argus-session-rotate --instance '$INSTANCE' --session /tmp/argus-rotate/session.json" 2>&1 | grep -v '^\[GIN\]'
  echo
  echo "以上只是计划。确认无误后加 --apply 重跑。"
  exit 0
fi

echo "▶ APPLY（实例 $INSTANCE）"
"${SSH[@]}" "cd $APP_DIR && ./argus-session-rotate --instance '$INSTANCE' --session /tmp/argus-rotate/session.json --apply --notify --actor 'weekly-rotate'" 2>&1 | grep -v '^\[GIN\]'

echo
echo "▶ 实例是否热加载（同一进程内重建，不该重启）"
"${SSH[@]}" '
  cd /data/program/app/argus_single 2>/dev/null || exit 0
  tail -400 server.log | grep -aE "热加载|交易管理器已热替换|价格监控器已热替换" | tail -5 | cut -c1-160
'

cat <<'NOTE'

⚠️ 到这一步只证明了「库里换了、进程重载了」，**没有**证明新凭证能下单。
   余额查询走的是 apiKey 原生通道，永远正常，不能拿来当证据。
   真正的证据只有一次成功下单：
     ssh 上去 grep -aE "开(long|short)成功|Login Timeout" server.log | tail
NOTE
