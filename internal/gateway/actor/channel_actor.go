package actor

import (
	"fmt"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
)

type PlayerResolver func(playerID int64) *protoactor.PID

// PlayerRouter 把“发给哪一个 player actor”从本地 PID 细节里抽出来。
// 当前它可以先由单进程 cluster/runtime 提供，后面再切到真正的 remote/cluster identity。
type PlayerRouter interface {
	Send(playerID int64, message interface{}) error
}

type Option func(*ChannelActor)

type LoginFrame struct {
	WorldID int64
	Account string
}

// ClientPlayerEnvelope 表示这条客户端消息应该投递给玩家私有 actor。
type ClientPlayerEnvelope struct {
	Payload any
}

// ClientWorldEnvelope 表示这条客户端消息应该投递给世界 actor。
type ClientWorldEnvelope struct {
	Payload any
}

type ChannelActor struct {
	connID         string
	worldPID       *protoactor.PID
	playerResolver PlayerResolver
	playerRouter   PlayerRouter

	playerID  int64
	playerPID *protoactor.PID
}

// New 为每条网关连接创建一个 channelActor。
// 它只保存连接在线态和已绑定的玩家身份，不持有玩家主数据。
func New(connID string, worldPID *protoactor.PID, playerResolver PlayerResolver, opts ...Option) *ChannelActor {
	actor := &ChannelActor{
		connID:         connID,
		worldPID:       worldPID,
		playerResolver: playerResolver,
	}
	for _, opt := range opts {
		opt(actor)
	}
	return actor
}

func Props(connID string, worldPID *protoactor.PID, playerResolver PlayerResolver, opts ...Option) *protoactor.Props {
	return protoactor.PropsFromProducer(func() protoactor.Actor {
		return New(connID, worldPID, playerResolver, opts...)
	})
}

func WithPlayerRouter(router PlayerRouter) Option {
	return func(actor *ChannelActor) {
		actor.playerRouter = router
	}
}

func (c *ChannelActor) Receive(ctx protoactor.Context) {
	switch msg := ctx.Message().(type) {
	case LoginFrame:
		// 登录第一跳仍然先交给 world，由 world 把账号解析成稳定的 playerID。
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

		// 连接一旦拿到稳定 playerID，后续优先按 identity 转发；
		// 本地 PID 解析只作为当前骨架未完全服务化前的兼容兜底。
		c.playerID = msg.PlayerID
		if c.playerResolver != nil {
			c.playerPID = c.playerResolver(msg.PlayerID)
		}
	case ClientPlayerEnvelope:
		if err := c.forwardPlayer(ctx, msg.Payload); err != nil {
			ctx.Send(ctx.Self(), fmt.Errorf("forward player message: %w", err))
		}
	case ClientWorldEnvelope:
		// 世界消息仍然可以直接进 world actor，例如登录链路第一跳之前的世界侧协同。
		ctx.Send(c.worldPID, msg.Payload)
	}
}

func (c *ChannelActor) forwardPlayer(ctx protoactor.Context, payload interface{}) error {
	if c.playerID == 0 {
		return fmt.Errorf("player identity not bound")
	}

	if c.playerRouter != nil {
		return c.playerRouter.Send(c.playerID, payload)
	}

	if c.playerPID == nil {
		return fmt.Errorf("player actor not available")
	}

	ctx.Send(c.playerPID, payload)
	return nil
}
