package actor

import (
	"fmt"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
	worldservice "overmind/internal/world/service"
)

type PlayerResolver func(playerID int64) *protoactor.PID

// PlayerRouter 把 world 到 player 的调用改成“按 playerID 找实体”。
// 这让 world 不再关心 player actor 当前到底落在哪个节点、哪个 PID 上。
type PlayerRouter interface {
	RequestFuture(playerID int64, message interface{}, timeout time.Duration) (protoactor.Future, error)
}

type Option func(*WorldActor)

type WorldActor struct {
	loginService   worldservice.LoginService
	playerResolver PlayerResolver
	playerRouter   PlayerRouter
}

// WorldActor 先承担“一 worldId 对应一个 actor”的粗粒度职责。
// 当前它主要负责 world 侧准入校验，以及把登录流程协调到 player actor。
func New(loginService worldservice.LoginService, playerResolver PlayerResolver, opts ...Option) *WorldActor {
	actor := &WorldActor{
		loginService:   loginService,
		playerResolver: playerResolver,
	}
	for _, opt := range opts {
		opt(actor)
	}
	return actor
}

func Props(loginService worldservice.LoginService, playerResolver PlayerResolver, opts ...Option) *protoactor.Props {
	return protoactor.PropsFromProducer(func() protoactor.Actor {
		return New(loginService, playerResolver, opts...)
	})
}

func WithPlayerRouter(router PlayerRouter) Option {
	return func(actor *WorldActor) {
		actor.playerRouter = router
	}
}

func (w *WorldActor) Receive(ctx protoactor.Context) {
	switch msg := ctx.Message().(type) {
	case clustermsg.WorldLoginReq:
		w.handleWorldLogin(ctx, msg)
	}
}

func (w *WorldActor) handleWorldLogin(ctx protoactor.Context, msg clustermsg.WorldLoginReq) {
	// world 先把账号解析成稳定的 playerID，确保后续都围绕玩家身份路由。
	playerID, _, err := w.loginService.ResolvePlayer(msg.WorldID, msg.Account)
	if err != nil {
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: err.Error()})
		return
	}

	request := clustermsg.PlayerLoginReq{
		PlayerID:   playerID,
		WorldID:    msg.WorldID,
		Account:    msg.Account,
		ConnID:     msg.ConnID,
		ChannelPID: msg.ChannelPID,
	}

	result, err := w.requestPlayer(ctx, playerID, request)
	if err != nil {
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: err.Error()})
		return
	}

	// WorldActor 自己不持有玩家主数据，所以这里只负责透传 player 侧结论。
	switch response := result.(type) {
	case clustermsg.PlayerLoginResp:
		ctx.Respond(response)
	case clustermsg.PlayerLoginRejected:
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: response.Reason})
	default:
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: "unexpected player login response"})
	}
}

func (w *WorldActor) requestPlayer(ctx protoactor.Context, playerID int64, request clustermsg.PlayerLoginReq) (interface{}, error) {
	if w.playerRouter != nil {
		future, err := w.playerRouter.RequestFuture(playerID, request, time.Second)
		if err != nil {
			return nil, err
		}
		return future.Result()
	}

	playerPID := w.playerResolver(playerID)
	if playerPID == nil {
		return nil, fmt.Errorf("player actor not available")
	}
	return ctx.RequestFuture(playerPID, request, time.Second).Result()
}
