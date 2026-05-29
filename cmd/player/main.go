package main

import (
	"context"
	"log"

	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
	playerapp "overmind/internal/player/app"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Services.Player.Name, cfg.Log.Level, cfg.Log.Encoding)

	// main 只保留“装配配置并启动应用”这一个入口职责。
	// 具体组件的持有与回收交给 PlayerApp，和 antares-main 的 Node 设计保持一致。
	if err := app.RunServer(context.Background(), playerapp.New(cfg)); err != nil {
		log.Fatal(err)
	}
}
