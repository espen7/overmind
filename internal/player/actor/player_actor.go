package actor

import (
	"strconv"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/cluster"

	clustermsg "overmind/internal/cluster/messages"
	playerservice "overmind/internal/player/service"
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

// PlayerActor 代表单个玩家的私有在线边界。
// 第一版先把登录绑定、唯一实例和空闲钝化收敛到这里，后续再继续承接背包、建筑、科技等主数据。
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
	}
}

func (p *PlayerActor) handlePlayerLogin(ctx protoactor.Context, msg clustermsg.PlayerLoginReq) {
	p.bindClusterIdentity(ctx)
	if msg.PlayerID != p.playerID {
		ctx.Respond(clustermsg.PlayerLoginRejected{Reason: "player actor identity mismatch"})
		return
	}

	login, err := p.loginService.Login(msg.PlayerID, msg.WorldID, msg.Account)
	if err != nil {
		ctx.Respond(clustermsg.PlayerLoginRejected{Reason: err.Error()})
		return
	}

	if p.channelPID != nil && p.connID != "" && p.connID != msg.ConnID {
		// 同一玩家重新登录时，旧连接会被显式标记为过期，
		// 避免两个 channelActor 同时认为自己还拥有这名玩家。
		ctx.Unwatch(p.channelPID)
		ctx.Send(p.channelPID, clustermsg.ChannelExpired{ConnID: p.connID})
	}

	p.connID = msg.ConnID
	p.channelPID = msg.ChannelPID
	ctx.Watch(msg.ChannelPID)

	// playerActor 接受这次绑定之后，才把最终成功结果回给 world/channel。
	ctx.Respond(clustermsg.PlayerLoginResp{
		PlayerID: login.PlayerID,
		WorldID:  login.WorldID,
		ConnID:   msg.ConnID,
	})
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
	return p.channelPID != nil
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
