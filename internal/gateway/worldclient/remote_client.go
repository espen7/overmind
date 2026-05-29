package worldclient

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	clustermsg "overmind/internal/cluster/messages"
	clusterruntime "overmind/internal/cluster/runtime"
	"overmind/internal/gateway/protocol"
	kitpb "overmind/pkg/pb/kit"
	worldpb "overmind/pkg/pb/world"
)

type Outbound struct {
	Recipients []int64
	Type       uint16
	Payload    []byte
}

type EnterSceneInput struct {
	PlayerID   int64
	PlayerName string
	SceneID    int64
	X          int32
	Y          int32
}

type RemoteClient struct {
	router  *clusterruntime.Router
	worldID int64
}

func NewRemoteClient(router *clusterruntime.Router, worldID int64) *RemoteClient {
	return &RemoteClient{
		router:  router,
		worldID: worldID,
	}
}

// EnterScene 把 gateway 持有的出生点上下文编码成 protobuf 请求，再包进 Envelope 发给 world。
// 这样 gateway 不再自己持有 world service，只负责把 portal 返回的进图参数透传给 world actor。
func (c *RemoteClient) EnterScene(input EnterSceneInput) ([]Outbound, error) {
	payload, err := proto.Marshal(&kitpb.WorldEnterSceneRequest{
		PlayerName: input.PlayerName,
		SceneId:    input.SceneID,
		X:          input.X,
		Y:          input.Y,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal enter scene request: %w", err)
	}

	return c.request(&kitpb.WorldRouteRequest{
		PlayerId: input.PlayerID,
		MsgType:  int32(protocol.MessageTypeEnterScene),
		Payload:  payload,
	})
}

func (c *RemoteClient) Move(playerID int64, req *worldpb.MoveRequest) ([]Outbound, error) {
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal move request: %w", err)
	}
	return c.request(&kitpb.WorldRouteRequest{
		PlayerId: playerID,
		MsgType:  int32(protocol.MessageTypeMoveRequest),
		Payload:  payload,
	})
}

func (c *RemoteClient) Attack(playerID int64, req *worldpb.AttackRequest) ([]Outbound, error) {
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal attack request: %w", err)
	}
	return c.request(&kitpb.WorldRouteRequest{
		PlayerId: playerID,
		MsgType:  int32(protocol.MessageTypeAttackRequest),
		Payload:  payload,
	})
}

func (c *RemoteClient) request(route *kitpb.WorldRouteRequest) ([]Outbound, error) {
	envelope, err := clustermsg.NewRouteEnvelope(route)
	if err != nil {
		return nil, err
	}
	future, err := c.router.RequestFuture(
		clusterruntime.WorldKind,
		clusterruntime.WorldIdentity(c.worldID),
		envelope,
		time.Second,
	)
	if err != nil {
		return nil, err
	}

	result, err := future.Result()
	if err != nil {
		return nil, fmt.Errorf("request world actor: %w", err)
	}

	reply, ok := result.(*kitpb.Envelope)
	if !ok {
		return nil, fmt.Errorf("unexpected world reply %T", result)
	}

	mesh := reply.GetMesh()
	if mesh != nil && mesh.GetCmd() == clustermsg.MeshCmdWorldError {
		return nil, clustermsg.DecodeErrorEnvelope(reply)
	}

	batch, err := clustermsg.DecodeDispatchEnvelope(reply)
	if err != nil {
		return nil, err
	}

	outbound := make([]Outbound, 0, len(batch.GetMessages()))
	for _, message := range batch.GetMessages() {
		outbound = append(outbound, Outbound{
			Recipients: message.GetRecipients(),
			Type:       uint16(message.GetMsgType()),
			Payload:    message.GetPayload(),
		})
	}
	return outbound, nil
}
