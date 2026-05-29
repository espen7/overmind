package actor

import (
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"
)

type DataManager interface {
	Init(system *protoactor.ActorSystem, self *protoactor.PID)
	Tick()
	Flush() bool
}

type ManagerFactory func(playerID int64) DataManager

type Option func(*Config)

type Config struct {
	IdleTimeout    time.Duration
	TickInterval   time.Duration
	ManagerFactory ManagerFactory
	OnPassivated   func(playerID int64)
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

type pendingMessage struct {
	message interface{}
	sender  *protoactor.PID
}

type playerInitialized struct{}

func (playerInitialized) NotInfluenceReceiveTimeout() {}

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
