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

	switch response := result.(type) {
	case clustermsg.PlayerLoginResp:
		ctx.Respond(response)
	case clustermsg.PlayerLoginRejected:
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: response.Reason})
	default:
		ctx.Respond(clustermsg.WorldLoginRejected{Reason: "unexpected player login response"})
	}
}
