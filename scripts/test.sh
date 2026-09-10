#!/usr/bin/env bash
#
# 跑 server/ 四个 Go 模块的测试，并把**默认被跳过的真实 MySQL 集成校验**一起跑起来。
#
# 为什么需要它：集成测试全部由 DSN 环境变量门控，没设就 t.Skip。日常 `go test ./...`
# 看着全绿，实际有 30+ 个用例根本没执行。2026-09-09 一次排查里，signal_slice
# 落 0 行和「同批重复键让整批 INSERT 失败」两个缺陷，都是现成的集成测试能抓到的，
# 只差一个 MySQL 容器——两个缺陷因此在生产里躺了很久。
#
# 用法：
#   scripts/test.sh              # 单元 + 集成（起一次性 MySQL 容器）
#   scripts/test.sh unit         # 只跑单元，不需要 docker
#   scripts/test.sh integration  # 只跑带 DSN 的集成用例
#   KEEP_DB=1 scripts/test.sh    # 跑完保留容器，便于手工查表
#
# 刻意不设 ARGUS_RUN_DEEPCOIN_INTEGRATION：那批用例打真实交易所接口，
# 其中若干会**真的下单**（common/utils/deepcoin_test.go 的注释也写着「谨慎测试
# 交易接口」）。CI 与本地默认都不该碰它，要验就人工单独跑。
set -euo pipefail

MODE="${1:-all}"
CONTAINER="${TEST_MYSQL_CONTAINER:-lt-test-mysql}"
PORT="${TEST_MYSQL_PORT:-13306}"
PASSWORD="lt-test"
IMAGE="${TEST_MYSQL_IMAGE:-mysql:8.0}"   # 可覆盖，便于验证「DB 起不来」这条路径

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULES=(common service manager-api argus_single)
# oracle 一个测试文件都没有，也没被其它模块 replace 引用，所以不进 MODULES；
# 但仍要编译，否则它会在没人察觉的情况下慢慢编不过。
BUILD_MODULES=(common service manager-api argus_single oracle)

# 三套集成用例各自一个库：它们都会 AutoMigrate 同名事实表，还会按 instance_key
# 删数据，共用一个库会互相踩。
DB_EVENTSTORE=lt_test_eventstore
DB_ARGUS_EVENT=lt_test_argus_event
DB_SIGNAL_BT=lt_test_signal_bt
# argus_config 的凭证轮换集成用例：会建表、按 it- 前缀的实例键增删自己的行。
DB_ARGUS_CONFIG=lt_test_argus_config

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[x]\033[0m %s\n' "$*" >&2; }

dsn_for() {
  printf 'root:%s@tcp(127.0.0.1:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local' \
    "$PASSWORD" "$PORT" "$1"
}

start_db() {
  if docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    log "复用已在运行的 $CONTAINER"
  else
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    log "启动 $IMAGE（容器 $CONTAINER，端口 $PORT）"
    docker run -d --name "$CONTAINER" \
      -e MYSQL_ROOT_PASSWORD="$PASSWORD" \
      -p "$PORT":3306 "$IMAGE" >/dev/null
  fi

  log "等待 MySQL 就绪"
  local i
  local tries="${TEST_MYSQL_TRIES:-60}"
  for i in $(seq 1 "$tries"); do
    if docker exec "$CONTAINER" mysql -uroot -p"$PASSWORD" -e 'SELECT 1' >/dev/null 2>&1; then
      log "MySQL 就绪（${i}00ms 内）"
      break
    fi
    if [ "$i" -eq "$tries" ]; then
      fail "MySQL $tries 次探测后仍不可用"
      docker logs --tail 30 "$CONTAINER" >&2 || true
      return 1
    fi
    sleep 1
  done

  local db
  for db in "$DB_EVENTSTORE" "$DB_ARGUS_EVENT" "$DB_SIGNAL_BT" "$DB_ARGUS_CONFIG"; do
    docker exec "$CONTAINER" mysql -uroot -p"$PASSWORD" \
      -e "CREATE DATABASE IF NOT EXISTS \`$db\` DEFAULT CHARACTER SET utf8mb4;" 2>/dev/null
  done
  log "已备好 4 个库：$DB_EVENTSTORE / $DB_ARGUS_EVENT / $DB_SIGNAL_BT / $DB_ARGUS_CONFIG"
}

stop_db() {
  if [ -n "${KEEP_DB:-}" ]; then
    warn "KEEP_DB 已设，保留容器 $CONTAINER（手工查完记得 docker rm -f $CONTAINER）"
    return
  fi
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  log "已销毁 $CONTAINER"
}

# build_all 先把五个模块都编一遍。编不过就没必要往下跑测试，
# 而且能覆盖 oracle 这种没有测试的模块。
build_all() {
  log "编译五个模块"
  local m rc=0
  for m in "${BUILD_MODULES[@]}"; do
    if (cd "$REPO_ROOT/server/$m" && go build ./... 2>&1 | head -20); then
      printf '    %-12s ok\n' "$m"
    else
      fail "$m 编译失败"
      rc=1
    fi
  done
  return $rc
}

# run_modules <标签>：按模块跑 go test，汇总 PASS/SKIP/FAIL。
#
# 带 DSN 时必须 -p 1 串行跑包：pkg/eventstore、pkg/eventstore/episode、
# pkg/eventstore/backfill 三个包都会对同一个库 AutoMigrate 同一批事实表，
# go test 默认并发跑包，两个 CREATE TABLE 撞一起就是
# Error 1050 Table already exists / Error 1146 doesn't exist。
run_modules() {
  local label="$1" rc=0 total_pass=0 total_skip=0
  log "$label"
  # -p 必须始终有值：bash 3.2（macOS 自带）在 set -u 下展开空数组会直接报
  # "unbound variable"，把整个 go test 打没，而外层只看到 PASS=0。
  local par
  par=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)
  [ -n "${EVENTSTORE_TEST_DSN:-}" ] && par=1
  local m out pass skip
  for m in "${MODULES[@]}"; do
    if ! out=$(cd "$REPO_ROOT/server/$m" && go test ./... -count=1 -p "$par" -v 2>&1); then
      rc=1
    fi
    pass=$(printf '%s\n' "$out" | grep -cE '^--- PASS' || true)
    skip=$(printf '%s\n' "$out" | grep -cE '^--- SKIP' || true)
    total_pass=$((total_pass + pass))
    total_skip=$((total_skip + skip))
    if printf '%s\n' "$out" | grep -qE '^--- FAIL|^FAIL'; then
      fail "$m 有失败用例："
      # 只列失败用例名 + 紧随其后的断言行；旧写法用 '\s+.*\.go:[0-9]+:' 通配，
      # 把 t.Log / t.Skipf 的输出也当成失败打出来，噪声盖过真信号。
      printf '%s\n' "$out" | awk '
        /^--- FAIL/ { print; infail=1; next }
        /^--- (PASS|SKIP)/ { infail=0; next }
        infail && /\.go:[0-9]+:/ { print }
      ' | head -40 >&2
      rc=1
    elif [ "$pass" -eq 0 ] && [ "$m" != "manager-api" ]; then
      # 一个测试都没跑成，通常说明 go test 自己挂了（比如脚本 set -u 报错
      # 把命令打没），而 FAIL grep 在空输出里什么也匹配不到，于是显示成绿的。
      # 这种「零测试」必须当失败，否则脚本会给出假绿。
      fail "$m 一个测试都没跑起来（PASS=0），原始输出前 20 行："
      printf '%s\n' "$out" | head -20 >&2
      rc=1
    else
      printf '    %-12s PASS=%-4s SKIP=%s\n' "$m" "$pass" "$skip"
    fi
  done
  printf '    %-12s PASS=%-4s SKIP=%s\n' "合计" "$total_pass" "$total_skip"
  return $rc
}

main() {
  cd "$REPO_ROOT"
  local rc=0

  build_all || exit 1

  if [ "$MODE" = "unit" ]; then
    unset EVENTSTORE_TEST_DSN ARGUS_EVENT_TEST_DSN SIGNAL_BACKTEST_TEST_DSN ARGUS_CONFIG_TEST_DSN
    run_modules "单元测试（不带 DSN，集成用例会 SKIP）" || rc=1
    exit $rc
  fi

  command -v docker >/dev/null || { fail "需要 docker；只跑单元测试请用：scripts/test.sh unit"; exit 2; }
  docker info >/dev/null 2>&1 || { fail "docker 守护进程不可用"; exit 2; }

  trap stop_db EXIT
  start_db || exit 2

  export EVENTSTORE_TEST_DSN="$(dsn_for "$DB_EVENTSTORE")"
  export ARGUS_EVENT_TEST_DSN="$(dsn_for "$DB_ARGUS_EVENT")"
  export SIGNAL_BACKTEST_TEST_DSN="$(dsn_for "$DB_SIGNAL_BT")"
  export ARGUS_CONFIG_TEST_DSN="$(dsn_for "$DB_ARGUS_CONFIG")"

  if [ "$MODE" = "integration" ]; then
    log "只跑集成用例"
    (cd "$REPO_ROOT/server/argus_single" && go test ./... -count=1 -p 1 -run 'Integration' -v) || rc=1
    (cd "$REPO_ROOT/server/service" && go test ./... -count=1 -p 1 -run 'Integration' -v) || rc=1
    exit $rc
  fi

  run_modules "全量测试（含真实 MySQL 集成校验）" || rc=1
  if [ $rc -eq 0 ]; then
    log "全部通过"
  else
    fail "有用例失败"
  fi
  exit $rc
}

main "$@"
