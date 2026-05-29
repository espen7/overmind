package app

import (
	"context"

	"github.com/asynkron/protoactor-go/cluster"

	clusterruntime "overmind/internal/cluster/runtime"
	gatewaynet "overmind/internal/gateway/net"
	"overmind/internal/gateway/playerclient"
	"overmind/internal/gateway/worldclient"
	platformapp "overmind/internal/platform/app"
	portalrepo "overmind/internal/portal/repository"
	portalservice "overmind/internal/portal/service"
	portaltransport "overmind/internal/portal/transport"
)

// GatewayApp 对齐 antares-main 的边缘接入定位：
// 这个进程只持有连接入口、portal 入口适配器，以及到 world 的 actor 远程代理。
// gateway 不再直接装配 world repository/service/handler。
type GatewayApp struct {
	cfg platformapp.Config

	runtime *clusterruntime.Runtime
	cluster *cluster.Cluster
	server  *gatewaynet.WSServer
}

func New(cfg platformapp.Config) *GatewayApp {
	return &GatewayApp{cfg: cfg}
}

func (a *GatewayApp) Start(ctx context.Context) error {
	a.runtime = clusterruntime.New(a.cfg.Actors.Gateway)
	a.cluster = clusterruntime.StartClusterClient(a.runtime.System(), a.cfg.Cluster, a.cfg.Actors.Gateway)
	router := clusterruntime.NewRouter(a.cluster)

	portalRepository := portalrepo.NewMemoryRepository()
	portalHandler := portaltransport.NewHandler(portalservice.New(portalRepository))
	playerClient := playerclient.NewRemoteClient(router)
	worldClient := worldclient.NewRemoteClient(router, 1)

	a.server = gatewaynet.NewWSServer(a.cfg.Services.Gateway.Address(), portalHandler, playerClient, worldClient)
	return a.server.Start(ctx)
}

func (a *GatewayApp) Stop(ctx context.Context) error {
	var firstErr error

	if a.server != nil {
		if err := a.server.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		a.server = nil
	}
	if a.cluster != nil {
		a.cluster.Shutdown(true)
		a.cluster = nil
	}
	a.runtime = nil

	return firstErr
}
