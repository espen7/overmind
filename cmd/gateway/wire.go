//go:build wireinject
// +build wireinject

package main

import (
	"fmt"
	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/wire"
	gatenet "overmind/internal/gateway/net"
	"overmind/internal/kit/config"
)

// ProvideActorSystem 封装 NewActorSystem
func ProvideActorSystem() *actor.ActorSystem {
	return actor.NewActorSystem()
}

// ProvideWSServer 提供 WebSocket 服务
func ProvideWSServer(system *actor.ActorSystem, cfg *config.Config) *gatenet.WSServer {
	return gatenet.NewWSServer(fmt.Sprintf(":%d", cfg.Server.Port), system)
}

// ProvideTCPServer 提供 TCP 服务 (端口+1)
func ProvideTCPServer(system *actor.ActorSystem, cfg *config.Config) *gatenet.TCPServer {
	// TCP port = WS Port + 1 (Just for example)
	return gatenet.NewTCPServer(fmt.Sprintf(":%d", cfg.Server.Port+1), system)
}

// ProviderSet 定义 Gateway 的所有依赖
var ProviderSet = wire.NewSet(
	ProvideActorSystem,
	ProvideWSServer,
	ProvideTCPServer,
	NewGatewayApp,
)

// InitializeGateway 是 Wire 的注入入口
func InitializeGateway(cfg *config.Config) (*GatewayApp, error) {
	wire.Build(ProviderSet)
	return &GatewayApp{}, nil
}
