package playerproxy

import (
	"time"

	clusterruntime "overmind/internal/cluster/runtime"
	kitpb "overmind/pkg/pb/kit"
)

// Proxy 是 player 侧访问另一名玩家实体的 shard proxy。
// 当前即使还没有挂具体业务消息，也先把“找另一个玩家实体”的调用方式收口。
type Proxy struct {
	router *clusterruntime.Router
}

func NewProxy(router *clusterruntime.Router) *Proxy {
	return &Proxy{router: router}
}

func (c *Proxy) RequestEnvelope(playerID int64, envelope *kitpb.Envelope) (*kitpb.Envelope, error) {
	return c.router.RequestEnvelope(
		clusterruntime.PlayerKind,
		clusterruntime.PlayerIdentity(playerID),
		envelope,
		time.Second,
	)
}
