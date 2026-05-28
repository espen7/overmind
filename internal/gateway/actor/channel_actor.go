package actor

import (
	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
)

type PlayerResolver func(playerID int64) *protoactor.PID

type LoginFrame struct {
	WorldID int64
	Account string
}

type ClientPlayerEnvelope struct {
	Payload any
}

type ClientWorldEnvelope struct {
	Payload any
}

type ChannelActor struct {
	connID         string
	worldPID       *protoactor.PID
	playerResolver PlayerResolver
	playerPID      *protoactor.PID
}

func New(connID string, worldPID *protoactor.PID, playerResolver PlayerResolver) *ChannelActor {
	return &ChannelActor{
		connID:         connID,
		worldPID:       worldPID,
		playerResolver: playerResolver,
	}
}

func Props(connID string, worldPID *protoactor.PID, playerResolver PlayerResolver) *protoactor.Props {
	return protoactor.PropsFromProducer(func() protoactor.Actor {
		return New(connID, worldPID, playerResolver)
	})
}

func (c *ChannelActor) Receive(ctx protoactor.Context) {
	switch msg := ctx.Message().(type) {
	case LoginFrame:
		ctx.RequestWithCustomSender(c.worldPID, clustermsg.WorldLoginReq{
			WorldID:    msg.WorldID,
			Account:    msg.Account,
			ConnID:     c.connID,
			ChannelPID: ctx.Self(),
		}, ctx.Self())
	case clustermsg.PlayerLoginResp:
		if msg.ConnID != c.connID {
			return
		}
		c.playerPID = c.playerResolver(msg.PlayerID)
	case ClientPlayerEnvelope:
		if c.playerPID != nil {
			ctx.Send(c.playerPID, msg.Payload)
		}
	case ClientWorldEnvelope:
		ctx.Send(c.worldPID, msg.Payload)
	}
}
