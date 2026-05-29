package worldclient

import (
	"time"

	clusterruntime "overmind/internal/cluster/runtime"
	kitpb "overmind/pkg/pb/kit"
)

// RemoteClient 是 player -> world 进程通信的统一出口。
// 后续 PlayerActor 如果需要向 world 发请求，统一从这里走 Envelope + protobuf。
type RemoteClient struct {
	router *clusterruntime.Router
}

func NewRemoteClient(router *clusterruntime.Router) *RemoteClient {
	return &RemoteClient{router: router}
}

func (c *RemoteClient) RequestEnvelope(worldID int64, envelope *kitpb.Envelope) (*kitpb.Envelope, error) {
	return c.router.RequestEnvelope(
		clusterruntime.WorldKind,
		clusterruntime.WorldIdentity(worldID),
		envelope,
		time.Second,
	)
}
