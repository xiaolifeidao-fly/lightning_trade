package db

import (
	"strings"
	"testing"

	repoa "common/middleware/db/internal/probea/repository"
	repob "common/middleware/db/internal/probeb/repository"
)

// probeRepository 只用于验证映射键的生成规则，不接数据库。
type probeRepository struct{}

// TestGetTypeNameIsPackageQualified 守的不变量：仓储单例的映射键必须带完整包路径。
//
// 只用短包名时，service/trade/repository 与 service/argus_event/repository 下
// 两个都叫 StrategyEventRepository 的类型会共用一个槽位，第二个调用方拿到
// 别人的实例、类型断言 panic，且直接打死 manager-api 进程。
func TestGetTypeNameIsPackageQualified(t *testing.T) {
	name := getTypeName[probeRepository]()
	if !strings.Contains(name, "common/middleware/db") {
		t.Fatalf("映射键必须含完整包路径，实际为 %q", name)
	}
	if !strings.HasSuffix(name, ".probeRepository") {
		t.Fatalf("映射键必须以类型名结尾，实际为 %q", name)
	}
}

// TestGetTypeNameDistinguishesSameNamedTypes 复现真实故障：两个短名相同、
// 包路径不同的仓储类型必须落在不同的映射键上。
//
// 这就是线上炸掉 manager-api 的那个形状——service/trade/repository 与
// service/argus_event/repository 各有一个 StrategyEventRepository（以及
// DevSampleRepository）。用 %T 算键时两者都是 "*repository.StrategyEventRepository"，
// 先注册的占坑，后来者 instance.(*R) panic 并打死进程。
func TestGetTypeNameDistinguishesSameNamedTypes(t *testing.T) {
	a := getTypeName[repoa.SameNameRepository]()
	b := getTypeName[repob.SameNameRepository]()
	if a == b {
		t.Fatalf("同名不同包的类型算出了相同的键 %q —— 单例映射会互相顶掉并 panic", a)
	}
	// 再走一遍真实路径：两个类型各取一次，不能因为键相同而串成同一个实例。
	if ra, rb := GetRepository[repoa.SameNameRepository](), GetRepository[repob.SameNameRepository](); ra == nil || rb == nil {
		t.Fatal("两种同名类型都应各自拿到实例")
	}
}

// TestGetRepositoryReturnsSameInstancePerType 单例语义不能因为改键而变。
func TestGetRepositoryReturnsSameInstancePerType(t *testing.T) {
	first := GetRepository[probeRepository]()
	second := GetRepository[probeRepository]()
	if first != second {
		t.Fatal("同一类型两次 GetRepository 应返回同一实例")
	}
}
