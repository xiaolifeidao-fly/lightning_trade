// argus-db-sync 把老库的行补齐到新库，用于 2026-09-16 的实例迁移收尾。
//
// 为什么可以按 id 增量：迁移快照保留了 id，同一 id 在两边是同一行。这一点不是
// 假设——逐表用 COUNT + BIT_XOR(CRC32(业务键)) 比对过重叠 id 区间，16 张表全部吻合。
// 所以增量 = SELECT * WHERE id > (目标库 max(id))。
//
// 为什么不用 mysqldump：服务器上没有 mysql 客户端，而同步必须在服务器上跑
// （本机跨公网到老库两次 "Lost connection"，昨天回灌也是 500 行 INSERT 19 秒后
// i/o timeout；同 VPC 内 72 天 3 分钟）。
//
// 为什么不手工拼 SQL 字面量：event_hash 是 binary 列，手工转义容易出错。这里
// SELECT * → 通用 scan → 参数化批量 INSERT，类型与转义都交给驱动。
//
// 三类表分开处理：
//
//	data   增量：按 id > 目标 max(id) 追加。9 张，约 38 万行。
//	config 整表替换：先 DELETE 再全量拷。7 张、每张 ≤9 行。必须整替换是因为
//	       argus_config_version 有唯一键 (instance_key, published_slot)——老库
//	       id=4 已 archived/slot=NULL、id=6 published/slot=1，而新库 id=4 还是
//	       published/slot=1，直接插 id=6 会撞唯一键。先删后拷绕开顺序问题。
//	       （已确认库里没有任何外键约束，删了不会级联。）
//	derived 不同步：episode / episode_entry 是派生表，argus-episode-rebuild
//	       只有整实例替换模式，同步完事件后重新派生即可。
//
// 用法照 cmd/argus-event-import 的先例：默认 dry run，加 --apply 才写库。
//
//	# 看要搬多少，不写库
//	go run ./cmd/argus-db-sync --from '<老DSN>' --to '<新DSN>'
//
//	# 真搬
//	go run ./cmd/argus-db-sync --from '<老DSN>' --to '<新DSN>' --apply
//
//	# 只比对两边（切换后验收）
//	go run ./cmd/argus-db-sync --from '<老DSN>' --to '<新DSN>' --verify
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// dataTables 走 id 增量。顺序按"先事件后派生物"排，便于中断后续跑。
var dataTables = []string{
	"strategy_event",
	"balance_sample",
	"dev_sample",
	"signal_slice",
	"trade_kline",
	"trade_backtest_batch",
	"trade_backtest_run",
	"trade_backtest_metric",
	"trade_backtest_trade",
}

// configTables 整表替换。argus_config_version 必须排在引用它的表之前删、之后插，
// 但既然没有外键，顺序只影响可读性。
var configTables = []string{
	"argus_config_version",
	"argus_config",
	"argus_account",
	"argus_account_risk",
	"argus_runtime_session",
	"argus_monitor_symbol",
	"argus_notification",
}

// derivedTables 不同步，仅在报告里点名，避免"漏了一张表"的错觉。
var derivedTables = []string{"episode", "episode_entry"}

func main() {
	from := flag.String("from", "", "源库 DSN（老库）")
	to := flag.String("to", "", "目标库 DSN（新库）")
	apply := flag.Bool("apply", false, "真正写库；缺省只做 dry run")
	verify := flag.Bool("verify", false, "只比对两边 COUNT/校验和，不写任何行")
	batchSize := flag.Int("batch-size", 500, "单条 INSERT 的行数")
	only := flag.String("only", "", "只处理这些表（逗号分隔），缺省按内置清单")
	flag.Parse()

	if strings.TrimSpace(*from) == "" || strings.TrimSpace(*to) == "" {
		log.Fatal("必须同时给 --from 与 --to")
	}
	src := open("源库", *from)
	defer src.Close()
	dst := open("目标库", *to)
	defer dst.Close()

	dataList, configList := dataTables, configTables
	if f := strings.TrimSpace(*only); f != "" {
		keep := map[string]bool{}
		for _, t := range strings.Split(f, ",") {
			keep[strings.TrimSpace(t)] = true
		}
		dataList = filter(dataList, keep)
		configList = filter(configList, keep)
	}

	if *verify {
		reportVerify(src, dst, append(append([]string{}, dataList...), configList...))
		return
	}

	mode := "dry run"
	if *apply {
		mode = "APPLY"
	}
	log.Printf("模式=%s  批大小=%d", mode, *batchSize)

	var total int64
	for _, t := range dataList {
		n := syncIncremental(src, dst, t, *batchSize, *apply)
		total += n
	}
	for _, t := range configList {
		n := syncFull(src, dst, t, *batchSize, *apply)
		total += n
	}

	log.Printf("合计 %d 行", total)
	if len(derivedTables) > 0 {
		log.Printf("未同步（派生表，需事后 argus-episode-rebuild 重建）: %s",
			strings.Join(derivedTables, ", "))
	}
	if !*apply {
		log.Print("dry run 结束；确认无误后加 --apply 重跑")
		return
	}
	log.Print("开始比对")
	reportVerify(src, dst, append(append([]string{}, dataList...), configList...))
}

func filter(list []string, keep map[string]bool) []string {
	var out []string
	for _, t := range list {
		if keep[t] {
			out = append(out, t)
		}
	}
	return out
}

func open(label, dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("打开%s失败: %v", label, err)
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatalf("连接%s失败: %v", label, err)
	}
	return db
}

// syncIncremental 把 id 大于目标库当前 max(id) 的行追加过去。
func syncIncremental(src, dst *sql.DB, table string, batch int, apply bool) int64 {
	maxID := scalarInt(dst, fmt.Sprintf("SELECT IFNULL(MAX(id),0) FROM `%s`", table))
	pending := scalarInt(src, fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE id > ?", table), maxID)
	log.Printf("[%s] 目标库 max(id)=%d，待搬 %d 行", table, maxID, pending)
	if pending == 0 || !apply {
		return pending
	}
	return streamCopy(src, dst, table,
		fmt.Sprintf("SELECT * FROM `%s` WHERE id > ? ORDER BY id", table), []any{maxID}, batch)
}

// syncFull 整表替换：先清空目标表，再全量拷过去。只用于极小的配置表。
func syncFull(src, dst *sql.DB, table string, batch int, apply bool) int64 {
	srcN := scalarInt(src, fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table))
	dstN := scalarInt(dst, fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table))
	log.Printf("[%s] 整表替换：目标 %d 行 → 源 %d 行", table, dstN, srcN)
	if !apply {
		return srcN
	}
	if _, err := dst.Exec(fmt.Sprintf("DELETE FROM `%s`", table)); err != nil {
		log.Fatalf("[%s] 清空目标表失败: %v", table, err)
	}
	return streamCopy(src, dst, table,
		fmt.Sprintf("SELECT * FROM `%s` ORDER BY id", table), nil, batch)
}

// streamCopy 流式读源、按 batch 行一条 INSERT 写目标。
// 通用 scan（*any）让驱动决定类型，[]byte 必须拷贝——RawBytes/驱动缓冲在
// 下一次 Next() 后就失效，直接攒着会读到串行的脏数据。
func streamCopy(src, dst *sql.DB, table, query string, args []any, batch int) int64 {
	rows, err := src.Query(query, args...)
	if err != nil {
		log.Fatalf("[%s] 读源库失败: %v", table, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		log.Fatalf("[%s] 取列名失败: %v", table, err)
	}

	var (
		buf     []any
		rowsBuf int
		copied  int64
		started = time.Now()
	)
	flush := func() {
		if rowsBuf == 0 {
			return
		}
		stmt := buildInsert(table, cols, rowsBuf)
		if _, err := dst.Exec(stmt, buf...); err != nil {
			log.Fatalf("[%s] 写目标库失败（已搬 %d 行）: %v", table, copied, err)
		}
		copied += int64(rowsBuf)
		buf, rowsBuf = buf[:0], 0
	}

	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			log.Fatalf("[%s] scan 失败: %v", table, err)
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				c := make([]byte, len(b))
				copy(c, b)
				vals[i] = c
			}
		}
		buf = append(buf, vals...)
		rowsBuf++
		if rowsBuf >= batch {
			flush()
			if copied%50000 == 0 {
				log.Printf("[%s]   已搬 %d 行（%.0fs）", table, copied, time.Since(started).Seconds())
			}
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("[%s] 遍历源库失败: %v", table, err)
	}
	flush()
	log.Printf("[%s] 完成 %d 行，用时 %.1fs", table, copied, time.Since(started).Seconds())
	return copied
}

// buildInsert 拼多行 INSERT。ON DUPLICATE KEY UPDATE 让整个同步可重复执行：
// 重跑既不会因主键/唯一键报错，也会把已存在行的内容纠正成源库的值。
// id 不进 UPDATE 列表——它是冲突判定的依据，改它没有意义。
func buildInsert(table string, cols []string, nRows int) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = "`" + c + "`"
	}
	one := "(" + strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + ")"
	tuples := make([]string, nRows)
	for i := range tuples {
		tuples[i] = one
	}
	var upd []string
	for _, c := range cols {
		if c == "id" {
			continue
		}
		upd = append(upd, fmt.Sprintf("`%s`=VALUES(`%s`)", c, c))
	}
	return fmt.Sprintf("INSERT INTO `%s` (%s) VALUES %s ON DUPLICATE KEY UPDATE %s",
		table, strings.Join(quoted, ","), strings.Join(tuples, ","), strings.Join(upd, ","))
}

// reportVerify 两边逐表比 COUNT 与 BIT_XOR(CRC32(id))。id 在两边保留，
// 所以这一对足以发现缺行/多行/错行；内容正确性由"直接整行拷贝"保证。
//
// 必须把"目标少行"和"目标多行"分开报：切换之后老库冻结、新库继续写，活跃表本来
// 就会目标多于源。最初版本两种都报"不一致"，切换后一跑满屏红字，而那恰恰是期望
// 状态——这种报告会诱导出错误的回滚决定。
func reportVerify(src, dst *sql.DB, tables []string) {
	fmt.Printf("\n%-24s %12s %12s %14s %14s  %s\n",
		"表", "源行数", "目标行数", "源校验和", "目标校验和", "结论")
	fmt.Println(strings.Repeat("-", 108))
	missing, ahead, mismatch := 0, 0, 0
	for _, t := range tables {
		q := fmt.Sprintf("SELECT COUNT(*), IFNULL(BIT_XOR(CRC32(id)),0) FROM `%s`", t)
		sn, sc := pairInt(src, q)
		dn, dc := pairInt(dst, q)
		var verdict string
		switch {
		case sn == dn && sc == dc:
			verdict = "一致"
		case dn < sn:
			verdict = fmt.Sprintf("*** 目标缺 %d 行 ***", sn-dn)
			missing++
		case dn > sn:
			verdict = fmt.Sprintf("目标多 %d 行（切换后新增，正常）", dn-sn)
			ahead++
		default:
			verdict = "*** 行数相同、校验和不同：错行 ***"
			mismatch++
		}
		fmt.Printf("%-24s %12d %12d %14d %14d  %s\n", t, sn, dn, sc, dc, verdict)
	}
	fmt.Println(strings.Repeat("-", 108))
	switch {
	case missing == 0 && mismatch == 0 && ahead == 0:
		fmt.Printf("全部一致（%d 张表）\n", len(tables))
	case missing == 0 && mismatch == 0:
		fmt.Printf("无缺行、无错行；%d 张表目标领先（切换后老库冻结、新库继续写，符合预期）\n", ahead)
	default:
		fmt.Printf("需要处理：%d 张缺行、%d 张错行（另有 %d 张目标领先属正常）\n",
			missing, mismatch, ahead)
	}
	fmt.Printf("未纳入比对（派生表）: %s\n\n", strings.Join(derivedTables, ", "))
}

func scalarInt(db *sql.DB, query string, args ...any) int64 {
	var v int64
	if err := db.QueryRow(query, args...).Scan(&v); err != nil {
		log.Fatalf("查询失败 %q: %v", query, err)
	}
	return v
}

func pairInt(db *sql.DB, query string) (int64, int64) {
	var a, b int64
	if err := db.QueryRow(query).Scan(&a, &b); err != nil {
		log.Fatalf("查询失败 %q: %v", query, err)
	}
	return a, b
}
