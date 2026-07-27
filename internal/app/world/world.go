package world

import (
	"fmt"
	"log"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"google.golang.org/protobuf/proto"

	"overmind/api/game"
	"overmind/api/rpc"
	"overmind/internal/pkg/actor"
	"overmind/internal/pkg/network"
	"overmind/internal/pkg/routing"
)

// WorldActor 大地图服务统一入口 (注册名: world_actor)。
// 阶段 1 形态：单节点单 Actor，不做 cell 分片。四向通信：
//
//	gate  → world: 网关转发客户端 world 协议段 (20000~29999) 的 RPCEnvelope
//	world → gate:  HandleMessage 的 from 即 ChannelActor PID, 直接回打包好的 WS 帧
//	home  → world: PlayerActor 投递内部协议段 (90000+) 的 RPCEnvelope
//	world → home:  按 home 哈希环定位归属节点, Call 协调器取 PID 后投递
//
// 内部逻辑按 "dispatch 入口 + 按坐标参数处理" 分层书写, 未来拆 cell Actor 时搬函数即可。
type WorldActor struct {
	act.Actor
	ringMgr *routing.Manager // home 哈希环 (world → home 主动通知寻址)
}

// Init 初始化大地图服务入口
func (w *WorldActor) Init(args ...any) error {
	if len(args) >= 1 {
		if mgr, ok := args[0].(*routing.Manager); ok {
			w.ringMgr = mgr
		}
	}
	log.Printf("[WorldActor] 大地图服务入口启动成功")
	return nil
}

// HandleMessage 统一 dispatch 入口: 解包 RPCEnvelope 后按协议号分发
func (w *WorldActor) HandleMessage(from gen.PID, message any) error {
	data, ok := message.([]byte)
	if !ok {
		log.Printf("[WorldActor] 收到未知类型消息: %T", message)
		return nil
	}

	envelope := &rpc.RPCEnvelope{}
	if err := proto.Unmarshal(data, envelope); err != nil {
		log.Printf("[WorldActor] 解包 RPCEnvelope 错误: %v", err)
		return nil
	}

	switch envelope.ProtoId {
	case int32(game.MsgID_MSG_C2S_WORLD_PING):
		// gate → world → gate: from 即网关 ChannelActor
		w.handleWorldPing(from, envelope)
	case actor.ProtoHomeBuildReport:
		// home → world → home: 收到玩家建筑上报, 回执确认
		w.handleBuildReport(envelope)
	default:
		log.Printf("[WorldActor] 未识别的协议 ID: %d (Player: %s)", envelope.ProtoId, envelope.PlayerId)
	}
	return nil
}

// handleWorldPing 处理客户端大地图探活 (演示 gate ↔ world 闭环)
func (w *WorldActor) handleWorldPing(gatePID gen.PID, envelope *rpc.RPCEnvelope) {
	req := &game.C2S_WorldPing{}
	if err := proto.Unmarshal(envelope.Payload, req); err != nil {
		log.Printf("[WorldActor] 解析 C2S_WorldPing 失败: %v", err)
		return
	}

	log.Printf("[WorldActor] 收到 Player %s 大地图探活, 坐标: (%d, %d)", envelope.PlayerId, req.X, req.Y)

	res := &game.S2C_WorldPong{
		ServerTime: time.Now().UnixNano(),
		X:          req.X,
		Y:          req.Y,
		Msg:        "world_alive",
	}
	resBytes, _ := proto.Marshal(res)
	resPacket, _ := network.PackWS(int32(game.MsgID_MSG_S2C_WORLD_PONG), 0, resBytes)

	// 回包给网关 ChannelActor ([]byte 分支直透客户端)
	if err := w.Send(gatePID, resPacket); err != nil {
		log.Printf("[WorldActor] 回包 WorldPong 给网关失败: %v", err)
	}
}

// handleBuildReport 处理 home 上报的建筑升级事件 (演示 home ↔ world 闭环)
func (w *WorldActor) handleBuildReport(envelope *rpc.RPCEnvelope) {
	log.Printf("[WorldActor] 收到 Player %s 建筑升级上报: %s", envelope.PlayerId, string(envelope.Payload))

	// 回执确认: 演示 world → home 主动投递通路
	ack := fmt.Sprintf("world_ack:%s", string(envelope.Payload))
	if err := w.sendToPlayer(envelope.PlayerId, actor.ProtoWorldBuildAck, []byte(ack)); err != nil {
		log.Printf("[WorldActor] 向 Player %s 投递建筑回执失败: %v", envelope.PlayerId, err)
	}
}

// sendToPlayer world → home 主动投递: 哈希环定位归属节点 → Call 协调器取 PID → 投递 envelope。
// 简化实现: 每次都同步解析 PID 且阻塞本 Actor (当前量级可接受), PID 缓存留作后续优化
func (w *WorldActor) sendToPlayer(playerID string, protoID int32, payload []byte) error {
	if w.ringMgr == nil {
		return fmt.Errorf("home_ring_not_configured")
	}

	targetNode := w.ringMgr.PickCurrent(playerID)
	if targetNode == "" {
		return fmt.Errorf("empty_home_ring")
	}

	coordinator := gen.ProcessID{Name: gen.Atom("home_coordinator"), Node: gen.Atom(targetNode)}
	// 超时对齐网关侧: 玩家不在线时协调器会走完整分配链 (释放握手 10s + 围栏抢占)
	resTerm, err := w.CallWithTimeout(coordinator, playerID, 15)
	if err != nil {
		return err
	}

	playerPID, ok := resTerm.(gen.PID)
	if !ok {
		return fmt.Errorf("resolve_player_failed: %v", resTerm)
	}

	return actor.SendEnvelope(w.Process, playerPID, 0, playerID, protoID, payload)
}

// Terminate 销毁处理
func (w *WorldActor) Terminate(reason error) {
	log.Printf("[WorldActor] 大地图服务入口正在终止: %v", reason)
}
