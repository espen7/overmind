package actor

import (
	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
	playerservice "overmind/internal/player/service"
)

type PlayerActor struct {
	loginService playerservice.LoginService
	connID       string
	channelPID   *protoactor.PID
}

func New(loginService playerservice.LoginService) *PlayerActor {
	return &PlayerActor{loginService: loginService}
}

func Props(loginService playerservice.LoginService) *protoactor.Props {
	return protoactor.PropsFromProducer(func() protoactor.Actor {
		return New(loginService)
	})
}

func (p *PlayerActor) Receive(ctx protoactor.Context) {
	switch msg := ctx.Message().(type) {
	case clustermsg.PlayerLoginReq:
		p.handlePlayerLogin(ctx, msg)
	}
}

func (p *PlayerActor) handlePlayerLogin(ctx protoactor.Context, msg clustermsg.PlayerLoginReq) {
	login, err := p.loginService.Login(msg.PlayerID, msg.WorldID, msg.Account)
	if err != nil {
		ctx.Respond(clustermsg.PlayerLoginRejected{Reason: err.Error()})
		return
	}

	if p.channelPID != nil && p.connID != "" && p.connID != msg.ConnID {
		ctx.Send(p.channelPID, clustermsg.ChannelExpired{ConnID: p.connID})
	}

	p.connID = msg.ConnID
	p.channelPID = msg.ChannelPID

	ctx.Respond(clustermsg.PlayerLoginResp{
		PlayerID: login.PlayerID,
		WorldID:  login.WorldID,
		ConnID:   msg.ConnID,
	})
}
