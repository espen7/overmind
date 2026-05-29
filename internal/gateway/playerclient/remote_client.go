package playerclient

import (
	"fmt"
	"time"

	clustermsg "overmind/internal/cluster/messages"
	clusterruntime "overmind/internal/cluster/runtime"
	kitpb "overmind/pkg/pb/kit"
)

type BindInput struct {
	PlayerID int64
	WorldID  int64
	Account  string
	ConnID   string
}

// RemoteClient 对应 antares-main 里 gateway 侧到 player 实体的远程代理。
// gateway 不持有玩家业务逻辑，只把“这条连接属于谁”的绑定关系投递给 player 节点。
type RemoteClient struct {
	router *clusterruntime.Router
}

func NewRemoteClient(router *clusterruntime.Router) *RemoteClient {
	return &RemoteClient{
		router: router,
	}
}

func (c *RemoteClient) BindSession(input BindInput) (*kitpb.PlayerBindResponse, error) {
	envelope, err := clustermsg.NewPlayerBindEnvelope(&kitpb.PlayerBindRequest{
		PlayerId: input.PlayerID,
		WorldId:  input.WorldID,
		Account:  input.Account,
		ConnId:   input.ConnID,
	})
	if err != nil {
		return nil, err
	}

	reply, err := c.request(input.PlayerID, envelope)
	if err != nil {
		return nil, err
	}
	if mesh := reply.GetMesh(); mesh != nil && mesh.GetCmd() == clustermsg.MeshCmdPlayerError {
		return nil, clustermsg.DecodePlayerErrorEnvelope(reply)
	}
	return clustermsg.DecodePlayerBindResponseEnvelope(reply)
}

func (c *RemoteClient) UnbindSession(playerID int64, connID string) error {
	envelope, err := clustermsg.NewPlayerUnbindEnvelope(&kitpb.PlayerUnbindRequest{
		PlayerId: playerID,
		ConnId:   connID,
	})
	if err != nil {
		return err
	}

	reply, err := c.request(playerID, envelope)
	if err != nil {
		return err
	}
	if mesh := reply.GetMesh(); mesh != nil && mesh.GetCmd() == clustermsg.MeshCmdPlayerError {
		return clustermsg.DecodePlayerErrorEnvelope(reply)
	}
	_, err = clustermsg.DecodePlayerUnbindResponseEnvelope(reply)
	return err
}

func (c *RemoteClient) request(playerID int64, message *kitpb.Envelope) (*kitpb.Envelope, error) {
	future, err := c.router.RequestFuture(
		clusterruntime.PlayerKind,
		clusterruntime.PlayerIdentity(playerID),
		message,
		time.Second,
	)
	if err != nil {
		return nil, err
	}

	result, err := future.Result()
	if err != nil {
		return nil, fmt.Errorf("request player actor: %w", err)
	}

	reply, ok := result.(*kitpb.Envelope)
	if !ok {
		return nil, fmt.Errorf("unexpected player reply %T", result)
	}
	return reply, nil
}
