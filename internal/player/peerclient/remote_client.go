package peerclient

import (
	"time"

	clusterruntime "overmind/internal/cluster/runtime"
	kitpb "overmind/pkg/pb/kit"
)

// RemoteClient 是 player -> player 进程通信的统一出口。
// 当前即使还没有挂具体业务消息，也先把“找另一个玩家实体”的调用方式收口。
type RemoteClient struct {
	router *clusterruntime.Router
}

func NewRemoteClient(router *clusterruntime.Router) *RemoteClient {
	return &RemoteClient{router: router}
}

func (c *RemoteClient) RequestEnvelope(playerID int64, envelope *kitpb.Envelope) (*kitpb.Envelope, error) {
	return c.router.RequestEnvelope(
		clusterruntime.PlayerKind,
		clusterruntime.PlayerIdentity(playerID),
		envelope,
		time.Second,
	)
}
