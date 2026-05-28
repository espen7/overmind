package runtime

import (
	"fmt"

	"github.com/asynkron/protoactor-go/actor"

	"overmind/internal/platform/app"
)

type Runtime struct {
	system *actor.ActorSystem
	root   *actor.RootContext
	config app.ActorConfig
}

func New(cfg app.ActorConfig) *Runtime {
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

func (r *Runtime) Address() string {
	return fmt.Sprintf("%s:%d", r.config.Host, r.config.Port)
}
