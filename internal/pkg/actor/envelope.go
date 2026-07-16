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

// SendEnvelope 封包并发送跨节点 RPC 消息
func SendEnvelope(process gen.Process, to gen.PID, sessionID int64, playerID string, protoID int32, payloadBytes []byte) error {
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
