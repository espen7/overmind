package actor

import (
	"fmt"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"
	"google.golang.org/protobuf/proto"

	clustermsg "overmind/internal/cluster/messages"
	"overmind/internal/gateway/protocol"
	worldrepo "overmind/internal/world/repository"
	worldservice "overmind/internal/world/service"
	kitpb "overmind/pkg/pb/kit"
	worldpb "overmind/pkg/pb/world"
)

type PlayerResolver func(playerID int64) *protoactor.PID

// PlayerRouter 把 world 到 player 的调用改成“按 playerID 找实体”。
// 这样 world 不再关心 player actor 当前到底落在哪个节点、哪个 PID 上。
type PlayerRouter interface {
	RequestFuture(playerID int64, message interface{}, timeout time.Duration) (protoactor.Future, error)
}

type Option func(*WorldActor)

type WorldActor struct {
	loginService   worldservice.LoginService
	playerResolver PlayerResolver
	playerRouter   PlayerRouter

	sceneService  *worldservice.SceneService
	combatService *worldservice.CombatService
	world         *worldrepo.MemoryWorld
}

// WorldActor 先承担“一个 worldId 对应一个 actor”的粗粒度职责。
// 这版除了保留登录协调，还承接 gateway 远程投递来的进图、移动、战斗请求。
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

// RemoteProps 给 world 进程注册 remote kind 使用。
// 当前 world 进程内的世界状态仍然先保持内存态，但边界已经切成“gateway 远程请求 -> world actor 执行”。
func ClusterProps(loginService worldservice.LoginService, scene *worldservice.SceneService, combat *worldservice.CombatService, world *worldrepo.MemoryWorld) *protoactor.Props {
	return Props(
		loginService,
		nil,
		WithSceneRuntime(scene, combat, world),
	)
}

func RemoteProps(loginService worldservice.LoginService, scene *worldservice.SceneService, combat *worldservice.CombatService, world *worldrepo.MemoryWorld) *protoactor.Props {
	return ClusterProps(loginService, scene, combat, world)
}

func WithPlayerRouter(router PlayerRouter) Option {
	return func(actor *WorldActor) {
		actor.playerRouter = router
	}
}

func WithSceneRuntime(scene *worldservice.SceneService, combat *worldservice.CombatService, world *worldrepo.MemoryWorld) Option {
	return func(actor *WorldActor) {
		actor.sceneService = scene
		actor.combatService = combat
		actor.world = world
	}
}

func (w *WorldActor) Receive(ctx protoactor.Context) {
	switch msg := ctx.Message().(type) {
	case clustermsg.WorldLoginReq:
		w.handleWorldLogin(ctx, msg)
	case *kitpb.Envelope:
		w.handleGatewayEnvelope(ctx, msg)
	}
}

func (w *WorldActor) handleWorldLogin(ctx protoactor.Context, msg clustermsg.WorldLoginReq) {
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

func (w *WorldActor) handleGatewayEnvelope(ctx protoactor.Context, envelope *kitpb.Envelope) {
	reply, err := w.dispatchGatewayEnvelope(ctx, envelope)
	if err != nil {
		ctx.Respond(clustermsg.NewErrorEnvelope(err.Error()))
		return
	}
	ctx.Respond(reply)
}

func (w *WorldActor) dispatchGatewayEnvelope(ctx protoactor.Context, envelope *kitpb.Envelope) (*kitpb.Envelope, error) {
	if w.sceneService == nil || w.combatService == nil || w.world == nil {
		return nil, fmt.Errorf("world runtime not initialized")
	}

	request, err := clustermsg.DecodeRouteEnvelope(envelope)
	if err != nil {
		return nil, err
	}

	switch uint16(request.GetMsgType()) {
	case protocol.MessageTypeEnterScene:
		return w.handleEnterScene(request.GetPlayerId(), request.GetPayload())
	case protocol.MessageTypeMoveRequest:
		return w.handleMove(request.GetPlayerId(), request.GetPayload())
	case protocol.MessageTypeAttackRequest:
		return w.handleAttack(request.GetPlayerId(), request.GetPayload())
	default:
		return nil, fmt.Errorf("unsupported world route message %d", request.GetMsgType())
	}
}

func (w *WorldActor) handleEnterScene(playerID int64, payload []byte) (*kitpb.Envelope, error) {
	var req kitpb.WorldEnterSceneRequest
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("unmarshal enter scene request: %w", err)
	}

	result, err := w.sceneService.Enter(playerID, req.GetPlayerName(), req.GetSceneId(), req.GetX(), req.GetY())
	if err != nil {
		return nil, err
	}

	responsePayload, err := proto.Marshal(worldservice.BuildSnapshot(result))
	if err != nil {
		return nil, fmt.Errorf("marshal scene snapshot: %w", err)
	}

	return clustermsg.NewDispatchEnvelope(&kitpb.WorldDispatchBatch{
		Messages: []*kitpb.WorldDispatchMessage{{
			Recipients: []int64{playerID},
			MsgType:    int32(protocol.MessageTypeSceneSnapshot),
			Payload:    responsePayload,
		}},
	})
}

func (w *WorldActor) handleMove(playerID int64, body []byte) (*kitpb.Envelope, error) {
	var req worldpb.MoveRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("unmarshal move request: %w", err)
	}

	result, err := w.sceneService.Move(playerID, req.GetX(), req.GetY())
	if err != nil {
		return nil, err
	}

	payload, err := proto.Marshal(worldservice.BuildMoveBroadcast(result))
	if err != nil {
		return nil, fmt.Errorf("marshal move broadcast: %w", err)
	}

	recipients := []int64{playerID}
	for _, player := range result.VisiblePlayers {
		recipients = append(recipients, player.ID)
	}

	return clustermsg.NewDispatchEnvelope(&kitpb.WorldDispatchBatch{
		Messages: []*kitpb.WorldDispatchMessage{{
			Recipients: recipients,
			MsgType:    int32(protocol.MessageTypeMoveBroadcast),
			Payload:    payload,
		}},
	})
}

func (w *WorldActor) handleAttack(playerID int64, body []byte) (*kitpb.Envelope, error) {
	var req worldpb.AttackRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("unmarshal attack request: %w", err)
	}

	player, err := w.world.Player(playerID)
	if err != nil {
		return nil, err
	}

	result, err := w.combatService.Attack(playerID, player.SceneID, req.GetTargetId())
	if err != nil {
		return nil, err
	}

	payload, err := proto.Marshal(worldservice.BuildCombatBroadcast(result))
	if err != nil {
		return nil, fmt.Errorf("marshal combat broadcast: %w", err)
	}

	recipients := []int64{playerID}
	for _, visible := range w.world.VisiblePlayers(player.SceneID, playerID) {
		recipients = append(recipients, visible.ID)
	}

	return clustermsg.NewDispatchEnvelope(&kitpb.WorldDispatchBatch{
		Messages: []*kitpb.WorldDispatchMessage{{
			Recipients: recipients,
			MsgType:    int32(protocol.MessageTypeCombatBroadcast),
			Payload:    payload,
		}},
	})
}
