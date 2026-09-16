package main

import (
	"strings"
	"testing"
)

// buildInsert 是这个工具里唯一的纯逻辑，也是唯一能"静默搬错数据"的地方：
// 占位符数量与 args 数量不匹配，驱动会直接报错（吵，安全）；但列名顺序错、
// 反引号漏掉、把 id 也放进 UPDATE 列表，都会**悄悄**写出错的行。所以这里逐条钉住。

func TestBuildInsertPlaceholderCountMatchesArgs(t *testing.T) {
	cols := []string{"id", "event_hash", "ts"}
	got := buildInsert("balance_sample", cols, 3)
	// 3 列 × 3 行 = 9 个占位符；与 streamCopy 攒的 args 数量必须一致，
	// 否则驱动报 "sql: expected N arguments"。
	if n := strings.Count(got, "?"); n != 9 {
		t.Fatalf("占位符 %d 个，期望 9：%s", n, got)
	}
	if n := strings.Count(got, "(?,?,?)"); n != 3 {
		t.Fatalf("行元组 %d 组，期望 3：%s", n, got)
	}
}

// 列名必须全部反引号：trade_kline 有一列叫 `interval`，是 MySQL 保留字，
// 不加反引号整条语句语法错误。
func TestBuildInsertQuotesReservedWordColumns(t *testing.T) {
	got := buildInsert("trade_kline", []string{"id", "interval", "open_time"}, 1)
	for _, want := range []string{"`interval`", "`open_time`", "`trade_kline`"} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少反引号形式 %s：%s", want, got)
		}
	}
	if strings.Contains(got, " interval ") || strings.Contains(got, ",interval,") {
		t.Errorf("interval 未被反引号包裹：%s", got)
	}
}

// id 不能进 ON DUPLICATE KEY UPDATE：它是冲突判定依据，
// 写 id=VALUES(id) 在按业务唯一键冲突时会试图改主键。
func TestBuildInsertExcludesIDFromUpdateList(t *testing.T) {
	got := buildInsert("strategy_event", []string{"id", "event_hash", "roi_pct"}, 1)
	upd := got[strings.Index(got, "ON DUPLICATE KEY UPDATE"):]
	if strings.Contains(upd, "`id`=VALUES(`id`)") {
		t.Errorf("id 不应出现在 UPDATE 列表：%s", upd)
	}
	for _, want := range []string{"`event_hash`=VALUES(`event_hash`)", "`roi_pct`=VALUES(`roi_pct`)"} {
		if !strings.Contains(upd, want) {
			t.Errorf("UPDATE 列表缺 %s：%s", want, upd)
		}
	}
}

// ON DUPLICATE KEY UPDATE 是"可重复执行"的全部依据：没有它，重跑同步
// 会在唯一键上报错中断，迁移收尾就没法分批/续跑。
func TestBuildInsertIsIdempotentForm(t *testing.T) {
	got := buildInsert("dev_sample", []string{"id", "event_hash"}, 2)
	if !strings.Contains(got, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("必须带 ON DUPLICATE KEY UPDATE 才能重跑：%s", got)
	}
}

// 列顺序必须与 rows.Columns() 给的顺序完全一致——streamCopy 就是按那个顺序
// 攒 args 的。这里钉住"按入参顺序输出，不做任何排序"。
func TestBuildInsertPreservesColumnOrder(t *testing.T) {
	got := buildInsert("t", []string{"id", "zzz", "aaa"}, 1)
	head := got[:strings.Index(got, "VALUES")]
	iID, iZ, iA := strings.Index(head, "`id`"), strings.Index(head, "`zzz`"), strings.Index(head, "`aaa`")
	if !(iID < iZ && iZ < iA) {
		t.Errorf("列顺序被改动了，应为 id,zzz,aaa：%s", head)
	}
}

// 单表单行也要成立（配置表只有几行）。
func TestBuildInsertSingleRow(t *testing.T) {
	got := buildInsert("argus_config", []string{"id", "config_version_id"}, 1)
	if n := strings.Count(got, "?"); n != 2 {
		t.Fatalf("占位符 %d 个，期望 2：%s", n, got)
	}
	if strings.Contains(got, "),(") {
		t.Fatalf("单行不该出现多元组：%s", got)
	}
}

func TestFilterKeepsOnlyRequested(t *testing.T) {
	got := filter([]string{"a", "b", "c"}, map[string]bool{"b": true})
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("期望 [b]，得到 %v", got)
	}
}
