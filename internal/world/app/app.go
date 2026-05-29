package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/asynkron/protoactor-go/cluster"

	clusterruntime "overmind/internal/cluster/runtime"
	platformapp "overmind/internal/platform/app"
	"overmind/internal/platform/logging"
	worldactor "overmind/internal/world/actor"
	worldrepo "overmind/internal/world/repository"
	worldservice "overmind/internal/world/service"
)

// WorldApp 统一持有 world 进程里的运行时组件：
// world repository、scene/combat service、remote actor 入口和健康检查服务都从这里装配。
type WorldApp struct {
	cfg platformapp.Config

	runtime      *clusterruntime.Runtime
	cluster      *cluster.Cluster
	httpServer   *http.Server
	httpListener net.Listener
}

func New(cfg platformapp.Config) *WorldApp {
	return &WorldApp{cfg: cfg}
}

func (a *WorldApp) Start(ctx context.Context) error {
	a.runtime = clusterruntime.New(a.cfg.Actors.World)

	world := worldrepo.NewMemoryWorld()
	scene := worldservice.NewScene(world)
	combat := worldservice.NewCombat(world)

	worldKind := cluster.NewKind(
		clusterruntime.WorldKind,
		worldactor.ClusterProps(
			worldservice.NewInMemoryLoginService(),
			scene,
			combat,
			world,
		),
	)
	a.cluster = clusterruntime.StartClusterMember(a.runtime.System(), a.cfg.Cluster, a.cfg.Actors.World, worldKind)
	return a.startHTTP()
}

func (a *WorldApp) Stop(ctx context.Context) error {
	var firstErr error

	if err := a.stopHTTP(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if a.cluster != nil {
		a.cluster.Shutdown(true)
		a.cluster = nil
	}
	a.runtime = nil
	return firstErr
}

func (a *WorldApp) startHTTP() error {
	listener, err := net.Listen("tcp", a.cfg.Services.World.Address())
	if err != nil {
		return fmt.Errorf("listen world health server: %w", err)
	}

	a.httpListener = listener
	a.httpServer = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("world ok:" + a.cfg.Actors.World.Address()))
		}),
	}

	go func() {
		if err := a.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.L().Error("world health server exited unexpectedly", logging.Error(err))
		}
	}()

	logging.L().Info("world service starting", logging.String("addr", listener.Addr().String()))
	return nil
}

func (a *WorldApp) stopHTTP(ctx context.Context) error {
	if a.httpServer == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := a.httpServer.Shutdown(shutdownCtx)
	a.httpServer = nil
	a.httpListener = nil
	return err
}
