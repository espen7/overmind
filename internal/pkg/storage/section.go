package storage

// Section 玩家数据分区接口，所有游戏玩法数据子结构必须实现
type Section interface {
	Key() string                         // 分区标识 key，对应 MongoDB 文档中的子文档字段名
	IsDirty() bool                       // 是否有未落地的修改
	ClearDirty()                         // 清除脏标记（flush 后调用）
	Marshal() ([]byte, error)            // 序列化为 JSON bytes
	Unmarshal(data []byte) error         // 从 JSON bytes 反序列化
	DirtyFields() map[string]interface{} // 返回脏字段的 JSON key -> 当前值（字段级 $set）
}
