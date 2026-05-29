package actor

import (
	"fmt"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
	kitpb "overmind/pkg/pb/kit"
)

type PlayerResolver func(playerID int64) *protoactor.PID

// PlayerRouter 把“发给哪个 player actor”从本地 PID 细节里抽出来。
// 后续无论是 gateway、world 还是 player 进程内发起请求，都走同一套 identity 路由心智。
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
		envelope, err := clustermsg.NewWorldLoginEnvelope(&kitpb.WorldLoginRequest{
			WorldId: msg.WorldID,
			Account: msg.Account,
			ConnId:  c.connID,
		})
		if err != nil {
			ctx.Send(ctx.Self(), fmt.Errorf("build world login envelope: %w", err))
			return
		}
		ctx.RequestWithCustomSender(c.worldPID, envelope, ctx.Self())

	case *kitpb.Envelope:
		c.handleEnvelope(ctx, msg)

	case clustermsg.PlayerLoginResp:
		// 兼容旧的本地测试和过渡链路。
		if msg.ConnID != c.connID {
			return
		}
		c.bindPlayer(msg.PlayerID)

	case ClientPlayerEnvelope:
		if err := c.forwardPlayer(ctx, msg.Payload); err != nil {
			ctx.Send(ctx.Self(), fmt.Errorf("forward player message: %w", err))
		}

	case ClientWorldEnvelope:
		ctx.Send(c.worldPID, msg.Payload)
	}
}

func (c *ChannelActor) handleEnvelope(ctx protoactor.Context, envelope *kitpb.Envelope) {
	mesh := envelope.GetMesh()
	if mesh == nil || mesh.GetCmd() != clustermsg.MeshCmdWorldLogin {
		return
	}

	response, err := clustermsg.DecodeWorldLoginResponseEnvelope(envelope)
	if err != nil {
		ctx.Send(ctx.Self(), fmt.Errorf("decode world login response: %w", err))
		return
	}
	if response.GetConnId() != c.connID {
		return
	}
	c.bindPlayer(response.GetPlayerId())
}

func (c *ChannelActor) bindPlayer(playerID int64) {
	c.playerID = playerID
	if c.playerResolver != nil {
		c.playerPID = c.playerResolver(playerID)
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
