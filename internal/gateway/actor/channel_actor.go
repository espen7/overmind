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

// ClientPlayerEnvelope 表示这条客户端消息应该被直投给 playerActor。
// 后续切到跨进程通信时，这里会被远程 Envelope/Letter 替换掉。
type ClientPlayerEnvelope struct {
	Payload any
}

// ClientWorldEnvelope 表示这条客户端消息应该被直投给 WorldActor。
type ClientWorldEnvelope struct {
	Payload any
}

type ChannelActor struct {
	connID         string
	worldPID       *protoactor.PID
	playerResolver PlayerResolver
	playerPID      *protoactor.PID
}

// New 为每条网关连接创建一个 channelActor。
// 它只保存“当前连接已经绑定到哪个 playerActor”这类在线态，不持有玩家主数据。
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
		// 登录第一跳先打到 world，由 world 完成账号到 playerID 的解析，
		// 再继续转给真正持有玩家主状态的 playerActor。
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
		// 只有登录成功后，channelActor 才会记住这个连接对应的 playerActor，
		// 后续客户端发来的“玩家私有消息”才能被继续转发。
		c.playerPID = c.playerResolver(msg.PlayerID)
	case ClientPlayerEnvelope:
		if c.playerPID != nil {
			ctx.Send(c.playerPID, msg.Payload)
		}
	case ClientWorldEnvelope:
		// 世界消息不要求先绑定 playerPID，例如进入场景前的一些 world 级协同消息。
		ctx.Send(c.worldPID, msg.Payload)
	}
}
