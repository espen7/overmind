package actor

import (
	"ergo.services/ergo/gen"
	"google.golang.org/protobuf/proto"

	"overmind/api/rpc"
)

// SpawnPlayerReq 用于网关与逻辑服协调器之间同步分配/创建 PlayerActor 的 RPC 请求体
type SpawnPlayerReq struct {
	PlayerID string
}

// 内部协议号 (不经网关/客户端, 仅用于 home ↔ world 节点间 RPCEnvelope 投递)
// 客户端协议段: 1000~1999 网关本地, 10000~19999 home, 20000~29999 world (见 game.proto);
// 90000+ 为服务端内部段, 网关不转发, 客户端无法伪造
const (
	ProtoHomeBuildReport int32 = 90001 // home → world: 玩家建筑升级上报
	ProtoWorldBuildAck   int32 = 90002 // world → home: 建筑上报确认回执
)

// SendEnvelope 封包并发送跨节点 RPC 消息
// to 支持 gen.PID / gen.ProcessID / gen.Atom 等 ergo 寻址类型
func SendEnvelope(process gen.Process, to any, sessionID int64, playerID string, protoID int32, payloadBytes []byte) error {
	// 1. 封装为 RPCEnvelope
	envelope := &rpc.RPCEnvelope{
		SessionId: sessionID,
		PlayerId:  playerID,
		ProtoId:   protoID,
		Payload:   payloadBytes,
	}

	// 2. 序列化 RPCEnvelope 为二进制
	envelopeBytes, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}

	// 3. 异步投递给目标 Actor
	return process.Send(to, envelopeBytes)
}
