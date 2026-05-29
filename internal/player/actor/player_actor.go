package actor

import (
	"strconv"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/cluster"

	clustermsg "overmind/internal/cluster/messages"
	playerservice "overmind/internal/player/service"
	kitpb "overmind/pkg/pb/kit"
)

type PlayerActor struct {
	playerID       int64
	loginService   playerservice.LoginService
	dataManager    DataManager
	managerFactory ManagerFactory
	idleTimeout    time.Duration
	tickInterval   time.Duration
	onPassivated   func(playerID int64)

	connID       string
	channelPID   *protoactor.PID
	pending      []pendingMessage
	initializing bool
	active       bool
	stopping     bool
	tickStop     chan struct{}
}

// PlayerActor 表示单个玩家的私有在线边界。
// 当前它同时兼容两条链路：
// 1. 旧的 world -> PlayerLoginReq 本地消息链路
// 2. 新的 gateway -> player Envelope 远程绑定/解绑链路
func New(playerID int64, loginService playerservice.LoginService, opts ...Option) *PlayerActor {
	config := defaultConfig()
	for _, opt := range opts {
		opt(&config)
	}

	return &PlayerActor{
		playerID:       playerID,
		loginService:   loginService,
		managerFactory: config.ManagerFactory,
		idleTimeout:    config.IdleTimeout,
		tickInterval:   config.TickInterval,
		onPassivated:   config.OnPassivated,
	}
}

func Props(playerID int64, loginService playerservice.LoginService, opts ...Option) *protoactor.Props {
	return protoactor.PropsFromProducer(func() protoactor.Actor {
		return New(playerID, loginService, opts...)
	})
}

// ClusterProps 给 cluster kind 注册用。
// 真正激活时，PlayerActor 会从 cluster identity 中恢复自己的 playerID。
func ClusterProps(loginService playerservice.LoginService, opts ...Option) *protoactor.Props {
	return protoactor.PropsFromProducer(func() protoactor.Actor {
		return New(0, loginService, opts...)
	})
}

func (p *PlayerActor) Receive(ctx protoactor.Context) {
	switch msg := ctx.Message().(type) {
	case *protoactor.Started:
		p.bindClusterIdentity(ctx)
		return
	case *protoactor.ReceiveTimeout:
		if p.active && !p.isOnline() {
			p.beginPassivation(ctx)
		}
	case *protoactor.Terminated:
		p.handleTerminated(msg)
	case *protoactor.Stopping:
		p.stopTicker()
	case *protoactor.Stopped:
		p.stopTicker()
		if p.onPassivated != nil {
			p.onPassivated(p.playerID)
		}
	case playerInitialized:
		p.handleInitialized(ctx)
	case playerInitializationFailed:
		p.handleInitializationFailed(ctx, msg)
	case playerTick:
		p.handleTick(ctx)
	case clustermsg.PlayerLoginReq:
		p.bindClusterIdentity(ctx)
		if p.playerID == 0 {
			p.playerID = msg.PlayerID
		}
		if p.shouldBuffer() {
			p.startInitialization(ctx)
			p.enqueue(ctx, msg)
			return
		}
		if p.stopping {
			ctx.Respond(clustermsg.PlayerLoginRejected{Reason: "player actor is stopping"})
			return
		}
		p.handlePlayerLogin(ctx, msg)
	case *kitpb.Envelope:
		p.handleGatewayEnvelope(ctx, msg)
	}
}

func (p *PlayerActor) handlePlayerLogin(ctx protoactor.Context, msg clustermsg.PlayerLoginReq) {
	p.bindClusterIdentity(ctx)
	if msg.PlayerID != p.playerID {
		ctx.Respond(clustermsg.PlayerLoginRejected{Reason: "player actor identity mismatch"})
		return
	}

	login, _, err := p.loginAndBind(ctx, msg.PlayerID, msg.WorldID, msg.Account, msg.ConnID, msg.ChannelPID)
	if err != nil {
		ctx.Respond(clustermsg.PlayerLoginRejected{Reason: err.Error()})
		return
	}

	ctx.Respond(clustermsg.PlayerLoginResp{
		PlayerID: login.PlayerID,
		WorldID:  login.WorldID,
		ConnID:   msg.ConnID,
	})
}

func (p *PlayerActor) handleGatewayEnvelope(ctx protoactor.Context, envelope *kitpb.Envelope) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope("player envelope missing mesh payload"))
		return
	}

	switch mesh.GetCmd() {
	case clustermsg.MeshCmdPlayerLogin:
		p.handlePlayerLoginEnvelope(ctx, envelope)
	case clustermsg.MeshCmdPlayerBind:
		p.handlePlayerBind(ctx, envelope)
	case clustermsg.MeshCmdPlayerUnbind:
		p.handlePlayerUnbind(ctx, envelope)
	default:
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope("unsupported player mesh command"))
	}
}

func (p *PlayerActor) handlePlayerLoginEnvelope(ctx protoactor.Context, envelope *kitpb.Envelope) {
	request, err := clustermsg.DecodePlayerLoginEnvelope(envelope)
	if err != nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope(err.Error()))
		return
	}

	p.bindClusterIdentity(ctx)
	if p.playerID == 0 {
		p.playerID = request.GetPlayerId()
	}
	if request.GetPlayerId() != p.playerID {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope("player actor identity mismatch"))
		return
	}
	if p.shouldBuffer() {
		p.startInitialization(ctx)
		p.enqueue(ctx, envelope)
		return
	}
	if p.stopping {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope("player actor is stopping"))
		return
	}

	login, expiredConnID, err := p.loginAndBind(ctx, request.GetPlayerId(), request.GetWorldId(), request.GetAccount(), request.GetConnId(), nil)
	if err != nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope(err.Error()))
		return
	}

	reply, err := clustermsg.NewPlayerLoginResponseEnvelope(&kitpb.PlayerLoginResponse{
		PlayerId:      login.PlayerID,
		WorldId:       login.WorldID,
		ConnId:        request.GetConnId(),
		ExpiredConnId: expiredConnID,
	})
	if err != nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope(err.Error()))
		return
	}
	ctx.Respond(reply)
}

func (p *PlayerActor) handlePlayerBind(ctx protoactor.Context, envelope *kitpb.Envelope) {
	request, err := clustermsg.DecodePlayerBindEnvelope(envelope)
	if err != nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope(err.Error()))
		return
	}

	p.bindClusterIdentity(ctx)
	if p.playerID == 0 {
		p.playerID = request.GetPlayerId()
	}
	if request.GetPlayerId() != p.playerID {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope("player actor identity mismatch"))
		return
	}
	if p.shouldBuffer() {
		p.startInitialization(ctx)
		p.enqueue(ctx, envelope)
		return
	}
	if p.stopping {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope("player actor is stopping"))
		return
	}

	login, expiredConnID, err := p.loginAndBind(ctx, request.GetPlayerId(), request.GetWorldId(), request.GetAccount(), request.GetConnId(), nil)
	if err != nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope(err.Error()))
		return
	}
	reply, err := clustermsg.NewPlayerBindResponseEnvelope(&kitpb.PlayerBindResponse{
		PlayerId:      login.PlayerID,
		WorldId:       login.WorldID,
		ConnId:        request.GetConnId(),
		ExpiredConnId: expiredConnID,
	})
	if err != nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope(err.Error()))
		return
	}
	ctx.Respond(reply)
}

func (p *PlayerActor) loginAndBind(ctx protoactor.Context, ctxPlayerID int64, worldID int64, account string, connID string, channelPID *protoactor.PID) (playerservice.LoginResult, string, error) {
	login, err := p.loginService.Login(ctxPlayerID, worldID, account)
	if err != nil {
		return playerservice.LoginResult{}, "", err
	}
	p.dataManager.OnLogin(login)
	expiredConnID := p.rebindConnection(ctx, connID, channelPID)
	return login, expiredConnID, nil
}

func (p *PlayerActor) handlePlayerUnbind(ctx protoactor.Context, envelope *kitpb.Envelope) {
	request, err := clustermsg.DecodePlayerUnbindEnvelope(envelope)
	if err != nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope(err.Error()))
		return
	}
	if request.GetPlayerId() != 0 && p.playerID != 0 && request.GetPlayerId() != p.playerID {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope("player actor identity mismatch"))
		return
	}

	if p.connID == request.GetConnId() {
		if p.channelPID != nil {
			ctx.Unwatch(p.channelPID)
			p.channelPID = nil
		}
		p.connID = ""
	}

	reply, err := clustermsg.NewPlayerUnbindResponseEnvelope(&kitpb.PlayerUnbindResponse{
		PlayerId: p.playerID,
		ConnId:   request.GetConnId(),
	})
	if err != nil {
		ctx.Respond(clustermsg.NewPlayerErrorEnvelope(err.Error()))
		return
	}
	ctx.Respond(reply)
}

func (p *PlayerActor) shouldBuffer() bool {
	return !p.active || p.initializing
}

func (p *PlayerActor) startInitialization(ctx protoactor.Context) {
	if p.initializing || p.active || p.stopping {
		return
	}

	if p.dataManager == nil {
		p.dataManager = p.managerFactory(p.playerID)
	}
	p.initializing = true
	p.dataManager.Init(ctx.ActorSystem(), ctx.Self())
}

func (p *PlayerActor) handleInitialized(ctx protoactor.Context) {
	if p.active || p.stopping {
		return
	}

	p.initializing = false
	p.active = true
	if p.idleTimeout > 0 {
		ctx.SetReceiveTimeout(p.idleTimeout)
	}
	p.startTicker(ctx.ActorSystem(), ctx.Self())
	p.replayPending(ctx)
}

func (p *PlayerActor) handleInitializationFailed(ctx protoactor.Context, msg playerInitializationFailed) {
	p.initializing = false
	p.stopping = true
	p.rejectPending(ctx, msg.Reason)
	ctx.Poison(ctx.Self())
}

func (p *PlayerActor) handleTick(ctx protoactor.Context) {
	p.dataManager.Tick()
	if p.stopping && p.dataManager.Flush() {
		ctx.Poison(ctx.Self())
	}
}

func (p *PlayerActor) beginPassivation(ctx protoactor.Context) {
	if p.stopping {
		return
	}

	p.stopping = true
	p.active = false
	ctx.CancelReceiveTimeout()
	if p.dataManager.Flush() {
		ctx.Poison(ctx.Self())
	}
}

func (p *PlayerActor) handleTerminated(msg *protoactor.Terminated) {
	if samePID(p.channelPID, msg.Who) {
		p.channelPID = nil
		p.connID = ""
	}
}

func (p *PlayerActor) isOnline() bool {
	return p.connID != ""
}

func (p *PlayerActor) enqueue(ctx protoactor.Context, message interface{}) {
	p.pending = append(p.pending, pendingMessage{
		message: message,
		sender:  ctx.Sender(),
	})
}

func (p *PlayerActor) replayPending(ctx protoactor.Context) {
	pending := p.pending
	p.pending = nil
	for _, item := range pending {
		if item.sender != nil {
			ctx.RequestWithCustomSender(ctx.Self(), item.message, item.sender)
			continue
		}
		ctx.Send(ctx.Self(), item.message)
	}
}

func (p *PlayerActor) rejectPending(ctx protoactor.Context, reason string) {
	pending := p.pending
	p.pending = nil
	for _, item := range pending {
		if item.sender != nil {
			if _, ok := item.message.(*kitpb.Envelope); ok {
				ctx.Send(item.sender, clustermsg.NewPlayerErrorEnvelope(reason))
				continue
			}
			ctx.Send(item.sender, clustermsg.PlayerLoginRejected{Reason: reason})
		}
	}
}

func (p *PlayerActor) startTicker(system *protoactor.ActorSystem, self *protoactor.PID) {
	if p.tickInterval <= 0 || p.tickStop != nil {
		return
	}

	p.tickStop = make(chan struct{})
	go func(stop <-chan struct{}) {
		ticker := time.NewTicker(p.tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				system.Root.Send(self, playerTick{})
			case <-stop:
				return
			}
		}
	}(p.tickStop)
}

func (p *PlayerActor) stopTicker() {
	if p.tickStop == nil {
		return
	}

	close(p.tickStop)
	p.tickStop = nil
}

func samePID(left *protoactor.PID, right *protoactor.PID) bool {
	if left == nil || right == nil {
		return false
	}

	return left.Address == right.Address && left.Id == right.Id
}

func (p *PlayerActor) bindClusterIdentity(ctx protoactor.Context) {
	if p.playerID != 0 {
		return
	}

	identity := cluster.GetClusterIdentity(ctx)
	if identity == nil {
		return
	}

	playerID, err := strconv.ParseInt(identity.Identity, 10, 64)
	if err != nil {
		return
	}
	p.playerID = playerID
}

func (p *PlayerActor) rebindConnection(ctx protoactor.Context, connID string, channelPID *protoactor.PID) string {
	expiredConnID := ""
	if p.connID != "" && p.connID != connID {
		expiredConnID = p.connID
		if p.channelPID != nil {
			ctx.Unwatch(p.channelPID)
			ctx.Send(p.channelPID, clustermsg.ChannelExpired{ConnID: p.connID})
		}
	}

	if p.channelPID != nil && channelPID == nil {
		ctx.Unwatch(p.channelPID)
	}

	p.connID = connID
	p.channelPID = channelPID
	if channelPID != nil {
		ctx.Watch(channelPID)
	}
	return expiredConnID
}
