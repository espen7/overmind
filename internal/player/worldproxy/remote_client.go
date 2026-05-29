package worldproxy

import (
	"time"

	clusterruntime "overmind/internal/cluster/runtime"
	kitpb "overmind/pkg/pb/kit"
)

// Proxy 是 player 侧访问 world 实体的 shard proxy。
// 后续 PlayerActor 如果需要向 world 发请求，统一从这里走 Envelope + protobuf。
type Proxy struct {
	router *clusterruntime.Router
}

func NewProxy(router *clusterruntime.Router) *Proxy {
	return &Proxy{router: router}
}

func (c *Proxy) RequestEnvelope(worldID int64, envelope *kitpb.Envelope) (*kitpb.Envelope, error) {
	return c.router.RequestEnvelope(
		clusterruntime.WorldKind,
		clusterruntime.WorldIdentity(worldID),
		envelope,
		time.Second,
	)
}
