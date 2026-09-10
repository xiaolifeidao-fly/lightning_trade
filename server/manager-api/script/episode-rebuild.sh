#!/usr/bin/env bash
# argus episode 派生表定时重建。
#
# 为什么需要：胜率、持仓生命周期这些指标都读 episode 派生表，而派生是手工步骤。
# 实测不跑就一直滞后——2026-09-10 早上滞后 9.7 小时，管理端「数据总览」会挂
# 滞后告警。这个脚本由 crontab 每 10 分钟拉一次。
#
# DSN 不写在这里：CLI 缺省会读同目录 configs/application.properties 的 sqlconn，
# 服务器上本来就有那份配置，不再复制一份凭证出来。
set -uo pipefail

APP_DIR=/data/program/app/manager-api
LOG=$APP_DIR/logs/episode-rebuild.log
INSTANCE=argus-single-roc

mkdir -p "$(dirname "$LOG")"

# 日志超过 5MB 就轮转一份，别把磁盘写满。
if [ -f "$LOG" ] && [ "$(stat -c %s "$LOG" 2>/dev/null || echo 0)" -gt 5242880 ]; then
  mv -f "$LOG" "$LOG.1"
fi

cd "$APP_DIR" || exit 1
# flock 防重入：派生是整表替换，两个进程同时跑会互相覆盖。
# -n = 拿不到锁就直接退出，不排队（下一个 10 分钟自然会再来）。
exec 9>"$APP_DIR/.episode-rebuild.lock"
if ! flock -n 9; then
  echo "$(date "+%F %T") 上一轮仍在运行，跳过" >> "$LOG"
  exit 0
fi

start=$(date +%s)
out=$(./argus-episode-rebuild --instance "$INSTANCE" --apply 2>&1)
rc=$?
cost=$(( $(date +%s) - start ))

if [ $rc -eq 0 ]; then
  # 只留结论行，别把整份 markdown 报告灌进日志
  echo "$(date "+%F %T") ok ${cost}s $(echo "$out" | grep -E "rebuilt|derived" | tr "\n" " ")" >> "$LOG"
else
  echo "$(date "+%F %T") FAILED rc=$rc ${cost}s" >> "$LOG"
  echo "$out" | tail -20 >> "$LOG"
fi
exit $rc
