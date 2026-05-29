package runtime

import (
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/cluster"
	"github.com/asynkron/protoactor-go/cluster/clusterproviders/automanaged"
	"github.com/asynkron/protoactor-go/cluster/identitylookup/disthash"
	"github.com/asynkron/protoactor-go/remote"

	platformapp "overmind/internal/platform/app"
)

// StartClusterMember 启动真正的 cluster member。
// 当前先用 protoactor-go 自带的 automanaged provider 做多进程开发环境，
// 把 gateway/player/world 从“手动 remote spawn”推进到“cluster identity 路由”。
func StartClusterMember(
	system *actor.ActorSystem,
	clusterCfg platformapp.ClusterConfig,
	nodeCfg platformapp.ActorNodeConfig,
	kinds ...*cluster.Kind,
) *cluster.Cluster {
	provider := automanaged.NewWithConfig(
		time.Duration(clusterCfg.RefreshTTLMS)*time.Millisecond,
		nodeCfg.DiscoveryPort,
		clusterCfg.Hosts...,
	)
	lookup := disthash.New()
	config := cluster.Configure(
		clusterCfg.Name,
		provider,
		lookup,
		remote.Configure(nodeCfg.Host, nodeCfg.Port),
		cluster.WithKinds(kinds...),
	)

	member := cluster.New(system, config)
	member.StartMember()
	return member
}

// StartClusterClient 启动 gateway 这类“只发请求、不承载实体”的 cluster client。
func StartClusterClient(
	system *actor.ActorSystem,
	clusterCfg platformapp.ClusterConfig,
	nodeCfg platformapp.ActorNodeConfig,
) *cluster.Cluster {
	provider := automanaged.NewWithConfig(
		time.Duration(clusterCfg.RefreshTTLMS)*time.Millisecond,
		nodeCfg.DiscoveryPort,
		clusterCfg.Hosts...,
	)
	lookup := disthash.New()
	config := cluster.Configure(
		clusterCfg.Name,
		provider,
		lookup,
		remote.Configure(nodeCfg.Host, nodeCfg.Port),
	)

	client := cluster.New(system, config)
	client.StartClient()
	return client
}
