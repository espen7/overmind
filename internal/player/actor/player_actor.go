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

// New PlayerActor 代表“单个玩家的在线私有边界”。
// 后续背包、建筑、科技、任务等玩家主数据都会优先收敛到这里。
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
		// 同一玩家重新登录时，旧连接会被显式标记为过期，
		// 避免两个 channelActor 同时认为自己还拥有这名玩家。
		ctx.Send(p.channelPID, clustermsg.ChannelExpired{ConnID: p.connID})
	}

	p.connID = msg.ConnID
	p.channelPID = msg.ChannelPID

	// playerActor 接受这次绑定之后，才把最终成功结果回给 world/channel。
	ctx.Respond(clustermsg.PlayerLoginResp{
		PlayerID: login.PlayerID,
		WorldID:  login.WorldID,
		ConnID:   msg.ConnID,
	})
}
