// Package repository 只为回归测试存在。
//
// 关键点：probea 与 probeb 下的包**都叫 repository**，且都定义
// SameNameRepository——这正是 service/trade/repository 与
// service/argus_event/repository 的形状。用 fmt.Sprintf("%%T") 算映射键时
// 两者都是 "*repository.SameNameRepository"，会共用同一个单例槽位。
package repository

// SameNameRepository 与另一个 probe 包里的类型短名完全相同。
type SameNameRepository struct{}
