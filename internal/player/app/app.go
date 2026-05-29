package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/asynkron/protoactor-go/cluster"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"

	clusterruntime "overmind/internal/cluster/runtime"
	platformapp "overmind/internal/platform/app"
	"overmind/internal/platform/logging"
	platformmongo "overmind/internal/platform/mongo"
	playeractor "overmind/internal/player/actor"
	playerrepo "overmind/internal/player/repository"
	playerservice "overmind/internal/player/service"
)

// PlayerApp 对齐 antares-main 的 Node/App 组合根思路：
// 由一个明确的应用对象持有并装配 mongo、actor runtime、cluster、repository 和 http server，
// 避免这些基础设施散落在 main 函数里互相耦合。
type PlayerApp struct {
	cfg platformapp.Config

	runtime      *clusterruntime.Runtime
	mongoClient  *mongodrv.Client
	database     *mongodrv.Database
	cluster      *cluster.Cluster
	httpServer   *http.Server
	httpListener net.Listener
}

func New(cfg platformapp.Config) *PlayerApp {
	return &PlayerApp{cfg: cfg}
}

// Start 按照“先依赖、后运行时、再入口”的顺序启动组件。
// 这样一旦底层依赖失败，进程会在启动阶段直接退出，不会留下半启动状态。
func (a *PlayerApp) Start(ctx context.Context) error {
	if err := a.startMongo(ctx); err != nil {
		return err
	}
	if err := a.startRuntime(); err != nil {
		_ = a.stopMongo(context.Background())
		return err
	}
	if err := a.startHTTP(); err != nil {
		_ = a.stopRuntime(context.Background())
		_ = a.stopMongo(context.Background())
		return err
	}
	return nil
}

// Stop 按照启动的逆序关闭组件。
// 这和 antares-main 里由 Node 统一回收组件的思路一致，区别只是我们当前先保留 Go 侧的简化实现。
func (a *PlayerApp) Stop(ctx context.Context) error {
	var firstErr error

	if err := a.stopHTTP(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := a.stopRuntime(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := a.stopMongo(ctx); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

func (a *PlayerApp) startMongo(ctx context.Context) error {
	client, database, err := platformmongo.Connect(ctx, a.cfg.Mongo)
	if err != nil {
		return err
	}

	a.mongoClient = client
	a.database = database
	return nil
}

func (a *PlayerApp) startRuntime() error {
	a.runtime = clusterruntime.New(a.cfg.Actor)

	playerRepository := playerrepo.NewMongoRepository(a.database)
	playerKind := cluster.NewKind(
		clusterruntime.PlayerKind,
		playeractor.ClusterProps(
			playerservice.NewStaticLoginService(),
			playeractor.WithManagerFactory(
				playeractor.NewMongoManagerFactory(
					playerRepository,
					playerrepo.NewPlayerActionRepository(playerRepository),
				),
			),
		),
	)

	// 当前先启动本地 virtual cluster，把业务边界收敛到 kind + identity。
	// 等后面接真正的 remote/provider 时，这层仍然由 PlayerApp 统一替换，不需要再改 main。
	a.cluster = clusterruntime.StartLocalVirtualCluster(a.runtime.System(), a.cfg.Actor, playerKind)
	return nil
}

func (a *PlayerApp) startHTTP() error {
	listener, err := net.Listen("tcp", a.cfg.Services.Player.Address())
	if err != nil {
		return fmt.Errorf("listen player health server: %w", err)
	}

	a.httpListener = listener
	a.httpServer = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("player ok:" + a.runtime.Address() + " db:" + a.database.Name()))
		}),
	}

	go func() {
		if err := a.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.L().Error("player health server exited unexpectedly", logging.Error(err))
		}
	}()

	logging.L().Info("player service starting", logging.String("addr", listener.Addr().String()))
	return nil
}

func (a *PlayerApp) stopHTTP(ctx context.Context) error {
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

func (a *PlayerApp) stopRuntime(ctx context.Context) error {
	if a.cluster == nil {
		return nil
	}

	// protoactor cluster 的 shutdown 当前不接收 context，这里先由 PlayerApp 负责统一调用。
	a.cluster.Shutdown(true)
	a.cluster = nil
	a.runtime = nil
	return nil
}

func (a *PlayerApp) stopMongo(ctx context.Context) error {
	if a.mongoClient == nil {
		return nil
	}

	disconnectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := a.mongoClient.Disconnect(disconnectCtx)
	a.mongoClient = nil
	a.database = nil
	return err
}
