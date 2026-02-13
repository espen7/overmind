package main

import (
	"context"
	"log"

	"time"

	"go.uber.org/zap"

	"overmind/internal/gateway/net"
	"overmind/internal/kit/config"
	"overmind/internal/kit/lifecycle"
	logger "overmind/internal/kit/log"
)

// GatewayApp 封装了应用程序的入口依赖，由 Wire 自动注入
type GatewayApp struct {
	servers []net.NetworkServer // 支持多种协议的服务 (WS, TCP 等)
}

// NewGatewayApp 是 GatewayApp 的构造函数
// 这里注入所有支持的 Server 实例
func NewGatewayApp(ws *net.WSServer, tcp *net.TCPServer) *GatewayApp {
	return &GatewayApp{
		servers: []net.NetworkServer{ws, tcp},
	}
}

func main() {
	// 0. 初始化配置
	cfg, err := config.Load(".")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 1. 初始化日志
	logger.Init(cfg.Log.Level, cfg.Log.Encoding == "json")
	logger.Info("Starting Gateway Service...",
		zap.String("env", cfg.Server.Env),
		zap.Int("port", cfg.Server.Port))

	// 2. 初始化依赖 (通过 Wire 生成的代码)
	app, err := InitializeGateway(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize gateway: %v", err)
	}

	// 2. 初始化生命周期管理器
	appHelper := lifecycle.New()

	// 注册服务启动/停止钩子
	appHelper.Append(lifecycle.Hook{
		Name: "Gateway Servers",
		OnStart: func(ctx context.Context) error {
			for _, srv := range app.servers {
				if err := srv.Start(ctx); err != nil {
					return err
				}
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// 在这里可以设置停止的超时时间
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			for _, srv := range app.servers {
				if err := srv.Stop(ctx); err != nil {
					logger.Error("Failed to stop server " + srv.Protocol() + ": " + err.Error())
				}
			}
			return nil
		},
	})

	// 3. 运行应用 (阻塞直到收到信号)
	if err := appHelper.Run(); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
