package repository

import "testing"

// TestNormalizeJSONColumn 守的是「json 类型的列永远不会收到空串」。
//
// 这条不变量断掉时的现象是 MySQL Error 3140（The document is empty），
// 整次 argus-config-import --apply 或一次 UI 存草稿直接失败。单测抓不到
// 数据库那一层，所以至少把归一函数本身钉住。
func TestNormalizeJSONColumn(t *testing.T) {
	for _, in := range []string{"", " ", "\t", "\n  "} {
		if got := normalizeJSONColumn(in); got != "null" {
			t.Fatalf("normalizeJSONColumn(%q) = %q，空白必须归一成 null", in, got)
		}
	}
	for _, in := range []string{"null", "{}", `{"a":1}`, "[]"} {
		if got := normalizeJSONColumn(in); got != in {
			t.Fatalf("normalizeJSONColumn(%q) = %q，非空值必须原样返回", in, got)
		}
	}
}

// TestBeforeSaveNormalizesEveryJSONColumn 确认三个 json 列都被钩子覆盖到，
// 漏掉任何一个都会在真实写库时炸。
func TestBeforeSaveNormalizesEveryJSONColumn(t *testing.T) {
	cfg := &ArgusConfig{}
	if err := cfg.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if cfg.ExtraConfigJSON != "null" {
		t.Fatalf("extra_config_json 未归一: %q", cfg.ExtraConfigJSON)
	}

	risk := &ArgusAccountRisk{}
	if err := risk.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if risk.TrailingStopTiersJSON != "null" {
		t.Fatalf("trailing_stop_tiers_json 未归一: %q", risk.TrailingStopTiersJSON)
	}
	if risk.ExtraRiskJSON != "null" {
		t.Fatalf("extra_risk_json 未归一: %q", risk.ExtraRiskJSON)
	}
}
