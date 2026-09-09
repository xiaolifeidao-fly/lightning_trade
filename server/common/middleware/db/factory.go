package db

import (
	"reflect"
	"sync"

	"gorm.io/gorm"
)

// 存储仓库实例的映射
var (
	repoInstances = make(map[string]interface{})
	repoMutex     sync.Mutex
)

func GetRepository[R any]() *R {
	repoType := getTypeName[R]()
	repoMutex.Lock()
	defer repoMutex.Unlock()

	// 检查是否已经存在实例
	if instance, exists := repoInstances[repoType]; exists {
		return instance.(*R)
	}

	// 创建新实例并保存到映射中
	var repoValue *R = new(R)
	if repo, ok := any(repoValue).(interface{ SetDb(*gorm.DB) }); ok {
		repo.SetDb(Db)
	}
	repoInstances[repoType] = repoValue
	return repoValue

}

// getTypeName 生成仓储单例的映射键，必须带完整包路径。
//
// 原来用 fmt.Sprintf("%T", new(R))，而 %T 只给**短包名**：
// service/trade/repository.StrategyEventRepository 与
// service/argus_event/repository.StrategyEventRepository
// 都被算成 "*repository.StrategyEventRepository"，两者共用一个 map 槽位。
// 后果是谁先 GetRepository 谁占坑，第二个取出来做 instance.(*R) 直接 panic：
//
//	interface conversion: interface {} is *repository.StrategyEventRepository,
//	not *repository.StrategyEventRepository (types from different packages)
//
// 实测在 manager-api 启动时炸掉整个进程（argus_event handler 先于 trade
// handler 注册，NewTradeService 随即 panic）。DevSampleRepository 同名同病。
// 单测覆盖不到，因为它要两个域的仓储在同一进程里都被取过才会触发。
func getTypeName[R any]() string {
	t := reflect.TypeOf(new(R)).Elem()
	// 匿名类型没有 PkgPath/Name，退回 String() 保证仍有唯一键。
	if pkg := t.PkgPath(); pkg != "" {
		return pkg + "." + t.Name()
	}
	return t.String()
}
