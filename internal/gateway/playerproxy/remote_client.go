package playerproxy

import (
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

// Proxy 对齐 Akka / antares-main 里的 shard proxy 语义。
// gateway 自己只是 cluster client，真正发往玩家实体时通过这个 player proxy 按 identity 转发。
type Proxy struct {
	router *clusterruntime.Router
}

func NewProxy(router *clusterruntime.Router) *Proxy {
	return &Proxy{
		router: router,
	}
}

func (c *Proxy) BindSession(input BindInput) (*kitpb.PlayerBindResponse, error) {
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

func (c *Proxy) UnbindSession(playerID int64, connID string) error {
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

func (c *Proxy) request(playerID int64, message *kitpb.Envelope) (*kitpb.Envelope, error) {
	reply, err := c.router.RequestEnvelope(
		clusterruntime.PlayerKind,
		clusterruntime.PlayerIdentity(playerID),
		message,
		time.Second,
	)
	if err != nil {
		return nil, err
	}
	return reply, nil
}
