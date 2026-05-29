package runtime

import (
	"fmt"

	"github.com/asynkron/protoactor-go/actor"

	"overmind/internal/platform/app"
)

type Runtime struct {
	system *actor.ActorSystem
	root   *actor.RootContext
	config app.ActorNodeConfig
}

// New 当前先只封装本地 ActorSystem，后面接 protoactor remote/cluster 时继续沿用这里做统一入口。
func New(cfg app.ActorNodeConfig) *Runtime {
	system := actor.NewActorSystem()
	return &Runtime{
		system: system,
		root:   system.Root,
		config: cfg,
	}
}

func (r *Runtime) System() *actor.ActorSystem {
	return r.system
}

func (r *Runtime) Root() *actor.RootContext {
	return r.root
}

// Address 返回当前 actor 节点预留给 remote/cluster 使用的监听地址。
func (r *Runtime) Address() string {
	return fmt.Sprintf("%s:%d", r.config.Host, r.config.Port)
}
