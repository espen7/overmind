package actor

import (
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	playerservice "overmind/internal/player/service"
	kitpb "overmind/pkg/pb/kit"
)

type DataManager interface {
	Init(system *protoactor.ActorSystem, self *protoactor.PID)
	Tick()
	Flush() bool
	OnLogin(login playerservice.LoginResult)
}

type ManagerFactory func(playerID int64) DataManager

// WorldProxy 约束 PlayerActor 到 world region 的 shard proxy 出口。
// 后续只要 player 侧有业务需要访问 world，都统一走 Envelope + protobuf。
type WorldProxy interface {
	RequestEnvelope(worldID int64, envelope *kitpb.Envelope) (*kitpb.Envelope, error)
}

// PlayerProxy 约束 PlayerActor 到其他玩家实体的 shard proxy 出口。
// 这样 player -> player 不会再绕回 PID 或裸 struct。
type PlayerProxy interface {
	RequestEnvelope(playerID int64, envelope *kitpb.Envelope) (*kitpb.Envelope, error)
}

type Option func(*Config)

type Config struct {
	IdleTimeout    time.Duration
	TickInterval   time.Duration
	ManagerFactory ManagerFactory
	OnPassivated   func(playerID int64)
	WorldProxy     WorldProxy
	PlayerProxy    PlayerProxy
}

func defaultConfig() Config {
	return Config{
		IdleTimeout:  time.Minute,
		TickInterval: time.Second,
		ManagerFactory: func(playerID int64) DataManager {
			return noopDataManager{}
		},
	}
}

func WithIdleTimeout(timeout time.Duration) Option {
	return func(cfg *Config) {
		cfg.IdleTimeout = timeout
	}
}

func WithTickInterval(interval time.Duration) Option {
	return func(cfg *Config) {
		cfg.TickInterval = interval
	}
}

func WithManagerFactory(factory ManagerFactory) Option {
	return func(cfg *Config) {
		cfg.ManagerFactory = factory
	}
}

func WithOnPassivated(fn func(playerID int64)) Option {
	return func(cfg *Config) {
		cfg.OnPassivated = fn
	}
}

func WithWorldProxy(proxy WorldProxy) Option {
	return func(cfg *Config) {
		cfg.WorldProxy = proxy
	}
}

func WithPlayerProxy(proxy PlayerProxy) Option {
	return func(cfg *Config) {
		cfg.PlayerProxy = proxy
	}
}

type pendingMessage struct {
	message interface{}
	sender  *protoactor.PID
}

type playerInitialized struct{}

func (playerInitialized) NotInfluenceReceiveTimeout() {}

type playerInitializationFailed struct {
	Reason string
}

func (playerInitializationFailed) NotInfluenceReceiveTimeout() {}

type playerTick struct{}

func (playerTick) NotInfluenceReceiveTimeout() {}

// noopDataManager 先提供一层与 antares-main 对齐的数据生命周期骨架：
// 激活时加载、tick 时追脏、钝化时 flush。
// 当前还没有真正接 Mongo，所以实现先保持为 no-op。
type noopDataManager struct{}

func (noopDataManager) Init(system *protoactor.ActorSystem, self *protoactor.PID) {
	system.Root.Send(self, playerInitialized{})
}

func (noopDataManager) Tick() {}

func (noopDataManager) Flush() bool { return true }

func (noopDataManager) OnLogin(login playerservice.LoginResult) {}
