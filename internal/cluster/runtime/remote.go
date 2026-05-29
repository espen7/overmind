package runtime

import (
	protoactor "github.com/asynkron/protoactor-go/actor"
	protoremote "github.com/asynkron/protoactor-go/remote"

	"overmind/internal/platform/app"
)

// StartRemote 启动 protoactor-go remote 监听，让不同进程的 actor 可以互相投递消息。
// 当前这层先承担 gateway/player/world 的跨进程通信，后面再继续往真正的 cluster provider 演进。
func StartRemote(system *protoactor.ActorSystem, cfg app.ActorNodeConfig, kinds ...*protoremote.Kind) *protoremote.Remote {
	remote := protoremote.NewRemote(system, protoremote.Configure(
		cfg.Host,
		cfg.Port,
		protoremote.WithKinds(kinds...),
	))
	remote.Start()
	return remote
}
