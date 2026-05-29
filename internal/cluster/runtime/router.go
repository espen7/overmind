package runtime

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/cluster"

	kitpb "overmind/pkg/pb/kit"
)

const (
	PlayerKind = "player"
	WorldKind  = "world"
)

func PlayerIdentity(playerID int64) string {
	return strconv.FormatInt(playerID, 10)
}

func WorldIdentity(worldID int64) string {
	return strconv.FormatInt(worldID, 10)
}

func WorldActorName(worldID int64) string {
	return fmt.Sprintf("world-%s", WorldIdentity(worldID))
}

func PlayerActorName(playerID int64) string {
	return fmt.Sprintf("player-%s", PlayerIdentity(playerID))
}

// Router 更接近一层通用的 shard/entity router。
// 它负责把“kind + identity”解析到唯一实体，具体业务再在上层包装成各自的 shard proxy。
type Router struct {
	mu      sync.RWMutex
	cluster *cluster.Cluster
}

func NewRouter(cluster *cluster.Cluster) *Router {
	return &Router{cluster: cluster}
}

// SetCluster 允许先创建 router、后在 App 完成 cluster 启动后回填真实实例。
// 这样 player/world 的 props 在装配阶段就能先拿到统一路由出口，不必退回到裸 PID。
func (r *Router) SetCluster(cluster *cluster.Cluster) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cluster = cluster
}

func (r *Router) PID(kind string, identity string) *actor.PID {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cluster == nil {
		return nil
	}
	return r.cluster.Get(identity, kind)
}

func (r *Router) Send(kind string, identity string, message interface{}) error {
	pid := r.PID(kind, identity)
	if pid == nil {
		return fmt.Errorf("identity %s/%s not available", kind, identity)
	}
	r.cluster.ActorSystem.Root.Send(pid, message)
	return nil
}

func (r *Router) RequestFuture(kind string, identity string, message interface{}, timeout time.Duration) (actor.Future, error) {
	if r == nil {
		return nil, fmt.Errorf("cluster router not initialized")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cluster == nil {
		return nil, fmt.Errorf("cluster router not initialized")
	}
	return r.cluster.RequestFuture(identity, kind, message, cluster.WithTimeout(timeout))
}

// RequestEnvelope 把“请求某个实体并期待 Envelope 响应”收口成统一入口。
// 这样 gateway/player/world 之间的跨进程通信都只围绕 kind + identity + protobuf envelope 展开。
func (r *Router) RequestEnvelope(kind string, identity string, envelope *kitpb.Envelope, timeout time.Duration) (*kitpb.Envelope, error) {
	future, err := r.RequestFuture(kind, identity, envelope, timeout)
	if err != nil {
		return nil, err
	}

	result, err := future.Result()
	if err != nil {
		return nil, err
	}

	reply, ok := result.(*kitpb.Envelope)
	if !ok {
		return nil, fmt.Errorf("unexpected envelope reply %T", result)
	}
	return reply, nil
}

type PlayerRouter struct {
	router *Router
}

func NewPlayerRouter(router *Router) *PlayerRouter {
	return &PlayerRouter{router: router}
}

func (r *PlayerRouter) Send(playerID int64, message interface{}) error {
	return r.router.Send(PlayerKind, PlayerIdentity(playerID), message)
}

// RequestEnvelope 把 world/player 对玩家实体的跨进程请求统一收口到 protobuf 信封。
func (r *PlayerRouter) RequestEnvelope(playerID int64, envelope *kitpb.Envelope, timeout time.Duration) (*kitpb.Envelope, error) {
	return r.router.RequestEnvelope(PlayerKind, PlayerIdentity(playerID), envelope, timeout)
}

func (r *PlayerRouter) RequestFuture(playerID int64, message interface{}, timeout time.Duration) (actor.Future, error) {
	return r.router.RequestFuture(PlayerKind, PlayerIdentity(playerID), message, timeout)
}

type WorldRouter struct {
	router *Router
}

func NewWorldRouter(router *Router) *WorldRouter {
	return &WorldRouter{router: router}
}

// RequestEnvelope 把 player 对 world 实体的跨进程请求统一收口到 protobuf 信封。
func (r *WorldRouter) RequestEnvelope(worldID int64, envelope *kitpb.Envelope, timeout time.Duration) (*kitpb.Envelope, error) {
	return r.router.RequestEnvelope(WorldKind, WorldIdentity(worldID), envelope, timeout)
}
