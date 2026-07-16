package world

import (
	"log"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// CellActor 代表一个空间大地图切片计算单元 (线程/协程隔离边界)
type CellActor struct {
	act.Actor
	cellID     int32
	chunkToken gen.Ref // 存储注册事件后返回的令牌
}

// Init 初始化 CellActor，并为其持有的 Chunks 注册原生 Ergo 事件总线
func (c *CellActor) Init(args ...any) error {
	c.cellID = 1
	log.Printf("[CellActor] Cell Actor %d 开始载入...", c.cellID)

	// 1. 注册该 Cell 下属 of Chunk 101 的事件总线，用于 AOI 订阅广播
	// RegisterEvent 返回 (gen.Ref, error)
	token, err := c.RegisterEvent(gen.Atom("chunk_101"), gen.EventOptions{})
	if err != nil {
		log.Printf("[CellActor] 注册 chunk_101 原生事件失败: %v", err)
		return err
	}
	c.chunkToken = token

	log.Printf("[CellActor] Cell Actor %d 启动成功，已成功注册 chunk_101 事件总线", c.cellID)
	return nil
}

// HandleCall 处理同步请求 (支持网关动态获取本 Cell PID)
func (c *CellActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "get_pid" {
		return c.PID(), nil
	}
	return "error: unhandled_call", nil
}

// HandleMessage 处理异步普通消息
func (c *CellActor) HandleMessage(from gen.PID, message any) error {
	// 例如：发生行军线变更，触发事件广播
	switch msg := message.(type) {
	case string:
		if msg == "trigger_march_sync" {
			log.Printf("[CellActor] 触发大地图行军事件广播...")
			// 使用 SendEvent 进行广播，入参: (事件名, 授权令牌, 消息载荷)
			err := c.SendEvent(gen.Atom("chunk_101"), c.chunkToken, "march_line_updated_payload")
			if err != nil {
				log.Printf("[CellActor] 广播事件失败: %v", err)
			}
		}
	}
	return nil
}

// Terminate 销毁处理
func (c *CellActor) Terminate(reason error) {
	log.Printf("[CellActor] Cell Actor %d 正在终止: %v", c.cellID, reason)
}
