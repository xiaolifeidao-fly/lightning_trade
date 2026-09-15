#!/usr/bin/env bash
#
# 把 DeepCoin BTC-USDT-SWAP 的 1m K 线按窗口回补进 trade_kline。
#
# 为什么不用 POST /klines/backfill(-range)：那条路只能向交易所要「最近 N 根」，
# klineFetchMax=1500 → 最多兜到 25 小时前，够不到更早的窗口（窗口太老时
# BackfillKlineRange 会标 Capped 直接放弃）。DeepCoin 的 /deepcoin/market/candles
# 支持 OKX 口径的 after 分页，能一直翻到 6 月，所以这里自己翻页。
#
# 时间口径：trade_kline.open_time 是 **UTC+8 墙钟**，与 strategy_event.ts 一致。
# 这一点用库里已有的 09-09 18:04 那根（open=78992.5）对上 DeepCoin 的
# 09-09 10:04 UTC 验过，并且首次回补时拿 1001 根重叠逐根比对：1000 根完全一致，
# 唯一差异那根是当初抓取时还没收盘的。
#
# 写入按唯一键 (platform_code, symbol, interval, open_time) 幂等，可重复执行。
#
# 用法：
#   scripts/backfill-klines-deepcoin.sh '2026-06-28 06:00' '2026-09-08 06:00'
set -euo pipefail

START_WALL="${1:?用法: $0 '起(UTC+8墙钟)' '止'}"
END_WALL="${2:?用法: $0 '起(UTC+8墙钟)' '止'}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROP="$ROOT/server/manager-api/configs/application.properties"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

command -v python3 >/dev/null || { echo "❌ 需要 python3" >&2; exit 1; }
command -v mysql >/dev/null || { echo "❌ 需要 mysql 客户端" >&2; exit 1; }

echo "▶ 抓取 DeepCoin 1m：$START_WALL → $END_WALL（UTC+8 墙钟）"
python3 - "$START_WALL" "$END_WALL" "$WORK" <<'PY'
import datetime as dt, json, os, sys, time, urllib.request

start_wall = dt.datetime.strptime(sys.argv[1], "%Y-%m-%d %H:%M")
end_wall   = dt.datetime.strptime(sys.argv[2], "%Y-%m-%d %H:%M")
work       = sys.argv[3]
INST, BAR, LIMIT = "BTC-USDT-SWAP", "1m", 1000

def wall_to_ms(w):
    return int((w - dt.timedelta(hours=8)).replace(tzinfo=dt.timezone.utc).timestamp() * 1000)
def ms_to_wall(ms):
    return dt.datetime.utcfromtimestamp(ms / 1000) + dt.timedelta(hours=8)

start_ms, after = wall_to_ms(start_wall), wall_to_ms(end_wall)
rows, page, empty_pages, skipped = {}, 0, 0, []
while after > start_ms and page < 400:
    page += 1
    url = (f"https://api.deepcoin.com/deepcoin/market/candles"
           f"?instId={INST}&bar={BAR}&limit={LIMIT}&after={after}")
    for attempt in range(4):
        try:
            data = json.load(urllib.request.urlopen(url, timeout=30))["data"]
            break
        except Exception as exc:                       # 交易所偶发 5xx / 连接重置
            if attempt == 3:
                print(f"  第 {page} 页连续 4 次失败，停止：{exc}", file=sys.stderr)
                data = []
                break
            time.sleep(1.5 * (attempt + 1))
    if not data:
        # 空页有两种：真的到头了，和交易所临时不返回。原先无条件 after -= 1000 分钟
        # 往回跳，两次之后 break——结果是**中间留洞而且不报**（首次跑 80 天时
        # 08-29~09-07 整 10 天就是这么丢的，全靠事后按日期比对才发现）。
        # 现在把跳过的区间记下来，最后显式报出来，让调用方按区间重抓。
        empty_pages += 1
        skipped.append((after - LIMIT * 60_000, after))
        if empty_pages >= 2:
            break
        after -= LIMIT * 60_000
        continue
    empty_pages = 0
    got = 0
    for r in data:
        ms = int(r[0])
        if ms < start_ms:
            continue
        if ms not in rows:
            rows[ms] = r
            got += 1
    oldest = min(int(r[0]) for r in data)
    if page % 10 == 0 or oldest <= start_ms:
        print(f"  第 {page:>3} 页：累计 {len(rows)} 根，最老 {ms_to_wall(oldest):%m-%d %H:%M}", file=sys.stderr)
    if oldest >= after:                                # 翻不动了，避免死循环
        break
    after = oldest

if not rows:
    sys.exit("没有抓到任何 K 线")
ordered = sorted(rows.items())
expected = int((end_wall - start_wall).total_seconds() // 60)
print(f"\n  共 {len(ordered)} 根，覆盖 {ms_to_wall(ordered[0][0]):%m-%d %H:%M} → "
      f"{ms_to_wall(ordered[-1][0]):%m-%d %H:%M}", file=sys.stderr)
print(f"  理应 ~{expected} 根，缺口 {expected - len(ordered)}", file=sys.stderr)

# 逐日核对：整天缺失是最隐蔽的失败（首次跑 80 天时丢了连续 10 天），
# 所以这里**显式报出来**，而不是让调用方事后去库里比对。
from collections import Counter
per_day = Counter(ms_to_wall(ms).date() for ms in rows)
d, end_d = start_wall.date(), end_wall.date()
holes = []
while d <= end_d:
    n = per_day.get(d, 0)
    full = 1440 if (d != start_wall.date() and d != end_d) else None
    if n == 0:
        holes.append((d, "整天缺失"))
    elif full and n < full - 60:
        holes.append((d, f"缺 {full - n} 分钟"))
    d += dt.timedelta(days=1)
if skipped:
    print("  ⚠️ 分页时跳过的区间（交易所返回空页）：", file=sys.stderr)
    for a, b in skipped:
        print(f"       {ms_to_wall(a):%m-%d %H:%M} ~ {ms_to_wall(b):%m-%d %H:%M}", file=sys.stderr)
if holes:
    print("  ⚠️ 覆盖不全的日期（按区间重跑本脚本即可补）：", file=sys.stderr)
    for d, why in holes:
        print(f"       {d} {why}", file=sys.stderr)
else:
    print("  逐日覆盖核对通过（无整天缺失、无大段空洞）", file=sys.stderr)

# 分片写 SQL：10 万行一个文件会让单条语句过大，也让失败时无从续跑。
CHUNK, PER_STMT = 20000, 500
for ci, base in enumerate(range(0, len(ordered), CHUNK)):
    part = ordered[base:base + CHUNK]
    path = os.path.join(work, f"klines_{ci:03d}.sql")
    with open(path, "w") as f:
        f.write("SET SESSION sql_mode='';\n")
        for i in range(0, len(part), PER_STMT):
            seg = part[i:i + PER_STMT]
            f.write("INSERT INTO trade_kline (active,created_time,updated_time,platform_code,symbol,"
                    "`interval`,open_time,close_time,open_price,high_price,low_price,close_price,"
                    "volume,turnover,trade_count) VALUES\n")
            f.write(",\n".join(
                f"(1,NOW(),NOW(),'deepcoin','BTCUSDT','1m','{ms_to_wall(ms):%Y-%m-%d %H:%M:%S}',"
                f"'1970-01-01 08:00:00',{float(r[1])},{float(r[2])},{float(r[3])},{float(r[4])},"
                f"{float(r[5])},{float(r[6])},0)" for ms, r in seg))
            f.write("\nON DUPLICATE KEY UPDATE open_price=VALUES(open_price),high_price=VALUES(high_price),"
                    "low_price=VALUES(low_price),close_price=VALUES(close_price),volume=VALUES(volume),"
                    "turnover=VALUES(turnover),updated_time=NOW();\n")
print(f"  已切成 {ci + 1} 个 SQL 分片", file=sys.stderr)
PY

DSN=$(grep -m1 '^sqlconn' "$PROP" | sed 's/^sqlconn[[:space:]]*=[[:space:]]*//' | tr -d ' ')
DBUSER=${DSN%%:*}; REST=${DSN#*:}; PASS=${REST%%@tcp(*}
HOSTPORT=${DSN#*@tcp(}; HOSTPORT=${HOSTPORT%%)*}
DBHOST=${HOSTPORT%%:*}; DBPORT=${HOSTPORT##*:}
DB=${DSN#*)/}; DB=${DB%%\?*}
run_sql() {
  for attempt in 1 2 3; do
    if MYSQL_PWD="$PASS" mysql -h"$DBHOST" -P"$DBPORT" -u"$DBUSER" -D"$DB" \
         --default-character-set=utf8mb4 --connect-timeout=20 < "$1" 2>"$WORK/err"; then
      return 0
    fi
    grep -qE "Lost connection|Can't connect|timed out" "$WORK/err" || { cat "$WORK/err" >&2; return 1; }
    sleep $((attempt * 3))
  done
  cat "$WORK/err" >&2; return 1
}

echo "▶ 写库（幂等，逐分片）"
n=0
for f in "$WORK"/klines_*.sql; do
  n=$((n + 1))
  run_sql "$f" || { echo "❌ 分片 $(basename "$f") 写入失败" >&2; exit 1; }
  echo "  分片 $n 完成"
done

echo "▶ 核对"
MYSQL_PWD="$PASS" mysql -h"$DBHOST" -P"$DBPORT" -u"$DBUSER" -D"$DB" -t --connect-timeout=20 -e "
SELECT COUNT(*) AS bars, MIN(open_time) AS mn, MAX(open_time) AS mx,
       ROUND(COUNT(*) / (TIMESTAMPDIFF(MINUTE, MIN(open_time), MAX(open_time)) + 1) * 100, 2) AS coverage_pct
FROM trade_kline WHERE platform_code='deepcoin' AND \`interval\`='1m';"
echo "✅ 完成"
