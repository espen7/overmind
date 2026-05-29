package actor

import (
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
	worldservice "overmind/internal/world/service"
)

type PlayerResolver func(playerID int64) *protoactor.PID

type WorldActor struct {
	loginService   worldservice.LoginService
	playerResolver PlayerResolver
}

// WorldActor 先承担“一个 worldId 对应一个 actor”的粗粒度职责。
// 第一版登录链路里，它主要负责 world 维度的准入校验和 playerActor 协调。
func New(loginService worldservice.LoginService, playerResolver PlayerResolver) *WorldActor {
	return &WorldActor{
		loginService:   loginService,
		playerResolver: playerResolver,
	}
}

func Props(loginService worldservice.LoginService, playerResolver PlayerResolver) *protoactor.Props {
	return protoactor.PropsFromProducer(func() protoactor.Actor {
		return New(loginService, playerResolver)
	})
}

func (w *WorldActor) Receive(ctx protoactor.Context) {
	switch msg := ctx.Message().(type) {
	case clustermsg.WorldLoginReq:
		w.handleWorldLogin(ctx, msg)
	}
}

func (w *WorldActor) handleWorldLogin(ctx protoactor.Context, msg clustermsg.WorldLoginReq) {
	// world 节点先把账号解析成确定的 playerID，确保后续都围绕稳定玩家身份继续路由。
	playerID, _, err := w.loginService.ResolvePlayer(msg.WorldID, msg.Account)
	if err != nil {
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: err.Error()})
		return
	}

	playerPID := w.playerResolver(playerID)
	if playerPID == nil {
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: "player actor not available"})
		return
	}

	result, err := ctx.RequestFuture(playerPID, clustermsg.PlayerLoginReq{
		PlayerID:   playerID,
		WorldID:    msg.WorldID,
		Account:    msg.Account,
		ConnID:     msg.ConnID,
		ChannelPID: msg.ChannelPID,
	}, time.Second).Result()
	if err != nil {
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: err.Error()})
		return
	}

	// WorldActor 自己不持有玩家主数据，所以这里只负责把 playerActor 的结论向上透传。
	switch response := result.(type) {
	case clustermsg.PlayerLoginResp:
		ctx.Respond(response)
	case clustermsg.PlayerLoginRejected:
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: response.Reason})
	default:
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: "unexpected player login response"})
	}
}
