package persistence

import "context"

// MemData 表示某一类 actor 内存数据的加载单元。
// 它只关心自己的初始化，不关心 actor 生命周期调度。
type MemData interface {
	Init(ctx context.Context) error
}

// TraceableMemData 在 MemData 的基础上增加脏追踪与刷盘能力。
// player/world DataManager 会统一调度这些实例。
type TraceableMemData interface {
	MemData
	MarkClean() error
	TraceEntities() error
	Flush(ctx context.Context) error
}
