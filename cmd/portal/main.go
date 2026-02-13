package main

import (
	"context"
	"log"
	"time"

	"go.uber.org/zap"

	"github.com/asynkron/protoactor-go/actor"
	"overmind/internal/kit/config"
	"overmind/internal/kit/lifecycle"
	logger "overmind/internal/kit/log"
	"overmind/internal/portal"
)

// PortalApp 封装了 Portal 服务的依赖
type PortalApp struct {
	System *actor.ActorSystem // ProtoActor 系统根
	Config *config.Config     // 全局配置
}

// NewPortalApp 构造函数
func NewPortalApp(system *actor.ActorSystem, cfg *config.Config) *PortalApp {
	return &PortalApp{
		System: system,
		Config: cfg,
	}
}

// Start 启动服务逻辑
func (app *PortalApp) Start() {
	// 3. Spawn Portal Actor
	props := actor.PropsFromProducer(func() actor.Actor {
		return portal.NewPortalActor()
	})
	pid, err := app.System.Root.SpawnNamed(props, "portal")
	if err != nil {
		log.Fatalf("Failed to spawn portal actor: %v", err)
	}

	logger.Info("Portal Actor started at " + pid.String())
}

func main() {
	// 0. 初始化配置
	cfg, err := config.Load(".")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 1. 初始化日志
	logger.Init("portal", cfg.Log.Encoding == "json")
	logger.Info("Starting Portal Service...",
		zap.String("env", cfg.Server.Env))

	// 2. 初始化依赖 (Wire)
	app, err := InitializePortal(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize portal: %v", err)
	}

	// 3. 初始化生命周期
	appHelper := lifecycle.New()

	appHelper.Append(lifecycle.Hook{
		Name: "Portal Service",
		OnStart: func(ctx context.Context) error {
			app.Start()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping Portal ActorSystem...")

			// Allow some time for graceful shutdown
			_, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			app.System.Shutdown()
			return nil
		},
	})

	// 4. 运行
	if err := appHelper.Run(); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
