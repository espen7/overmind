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
	Auth    ServiceConfig `mapstructure:"auth"`
	Game    ServiceConfig `mapstructure:"game"`
}

type SceneConfig struct {
	Width    int32 `mapstructure:"width"`
	Height   int32 `mapstructure:"height"`
	CellSize int32 `mapstructure:"cell_size"`
}

type GameConfig struct {
	Scene SceneConfig `mapstructure:"scene"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

type Config struct {
	Services Services   `mapstructure:"services"`
	Game     GameConfig `mapstructure:"game"`
	Log      LogConfig  `mapstructure:"log"`
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
	if err := check("auth", c.Services.Auth); err != nil {
		return err
	}
	if err := check("game", c.Services.Game); err != nil {
		return err
	}
	if c.Game.Scene.CellSize <= 0 {
		return fmt.Errorf("game.scene.cell_size must be positive")
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
