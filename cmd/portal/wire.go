//go:build wireinject
// +build wireinject

package main

import (
	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/wire"
	"overmind/internal/kit/config"
)

// ProvideActorSystem 封装
func ProvideActorSystem() *actor.ActorSystem {
	return actor.NewActorSystem()
}

// ProviderSet 定义 Portal 的所有依赖
var ProviderSet = wire.NewSet(
	ProvideActorSystem,
	NewPortalApp,
)

// InitializePortal 是 Wire 的注入入口
func InitializePortal(cfg *config.Config) (*PortalApp, error) {
	wire.Build(ProviderSet)
	return &PortalApp{}, nil
}
