package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type ServiceConfig struct {
	Name string `mapstructure:"name"`
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

func (s ServiceConfig) Address() string {
	host := s.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, s.Port)
}

type Services struct {
	Gateway ServiceConfig `mapstructure:"gateway"`
	Portal  ServiceConfig `mapstructure:"portal"`
	Player  ServiceConfig `mapstructure:"player"`
	World   ServiceConfig `mapstructure:"world"`
}

type ActorConfig struct {
	System string `mapstructure:"system"`
	Host   string `mapstructure:"host"`
	Port   int    `mapstructure:"port"`
}

type MongoConfig struct {
	URI      string `mapstructure:"uri"`
	Database string `mapstructure:"database"`
}

type SceneConfig struct {
	Width    int32 `mapstructure:"width"`
	Height   int32 `mapstructure:"height"`
	CellSize int32 `mapstructure:"cell_size"`
}

type WorldRuntimeConfig struct {
	Scene SceneConfig `mapstructure:"scene"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

type Config struct {
	Services Services           `mapstructure:"services"`
	Actor    ActorConfig        `mapstructure:"actor"`
	Mongo    MongoConfig        `mapstructure:"mongo"`
	World    WorldRuntimeConfig `mapstructure:"world"`
	Log      LogConfig          `mapstructure:"log"`
}

func (c Config) Validate() error {
	check := func(name string, svc ServiceConfig) error {
		if svc.Name == "" {
			return fmt.Errorf("%s service name is required", name)
		}
		if svc.Port <= 0 {
			return fmt.Errorf("%s service port must be positive", name)
		}
		return nil
	}

	if err := check("gateway", c.Services.Gateway); err != nil {
		return err
	}
	if err := check("portal", c.Services.Portal); err != nil {
		return err
	}
	if err := check("player", c.Services.Player); err != nil {
		return err
	}
	if err := check("world", c.Services.World); err != nil {
		return err
	}
	if c.Actor.System == "" {
		return fmt.Errorf("actor.system is required")
	}
	if c.Actor.Port <= 0 {
		return fmt.Errorf("actor.port must be positive")
	}
	if c.Mongo.URI == "" {
		return fmt.Errorf("mongo.uri is required")
	}
	if c.Mongo.Database == "" {
		return fmt.Errorf("mongo.database is required")
	}
	if c.World.Scene.CellSize <= 0 {
		return fmt.Errorf("world.scene.cell_size must be positive")
	}
	return nil
}

type Server interface {
	Start(context.Context) error
	Stop(context.Context) error
}

func RunServer(ctx context.Context, server Server) error {
	if err := server.Start(ctx); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigCh:
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Stop(stopCtx)
}

func RunHTTP(ctx context.Context, server *http.Server) error {
	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigCh:
	case err := <-errCh:
		return err
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(stopCtx)
}
