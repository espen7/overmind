package runtime

import (
	"fmt"
	"sync"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/cluster"
	"github.com/asynkron/protoactor-go/eventstream"
	"github.com/asynkron/protoactor-go/remote"

	"overmind/internal/platform/app"
)

// StartLocalVirtualCluster 启动一个单进程的 protoactor cluster。
// 它不是最终的跨节点 provider，只是把代码结构先拉到“kind + identity”的路由模型上，
// 方便后面无缝切到真正的 etcd/consul/memberlist provider。
func StartLocalVirtualCluster(system *actor.ActorSystem, cfg app.ActorConfig, kinds ...*cluster.Kind) *cluster.Cluster {
	provider := newLocalProvider()
	lookup := newLocalIdentityLookup()
	config := cluster.Configure(
		cfg.System,
		provider,
		lookup,
		remote.Configure(cfg.Host, cfg.Port),
		cluster.WithKinds(kinds...),
	)

	virtualCluster := cluster.New(system, config)
	virtualCluster.StartMember()
	return virtualCluster
}

type localProvider struct {
	cluster *cluster.Cluster
	self    *cluster.Member
}

func newLocalProvider() *localProvider {
	return &localProvider{}
}

func (p *localProvider) StartMember(c *cluster.Cluster) error {
	host, port, err := c.ActorSystem.GetHostPort()
	if err != nil {
		return err
	}

	p.cluster = c
	p.self = &cluster.Member{
		Host:  host,
		Port:  int32(port),
		Id:    fmt.Sprintf("%s@%s:%d", c.Config.Name, host, port),
		Kinds: c.GetClusterKinds(),
	}
	c.MemberList.UpdateClusterTopology(cluster.Members{p.self})
	return nil
}

func (p *localProvider) StartClient(c *cluster.Cluster) error {
	return p.StartMember(c)
}

func (p *localProvider) Shutdown(graceful bool) error {
	return nil
}

type localIdentityLookup struct {
	cluster        *cluster.Cluster
	terminatingSub *eventstream.Subscription

	mu          sync.Mutex
	activations map[string]*actor.PID
}

func newLocalIdentityLookup() *localIdentityLookup {
	return &localIdentityLookup{
		activations: make(map[string]*actor.PID),
	}
}

func (l *localIdentityLookup) Get(identity *cluster.ClusterIdentity) *actor.PID {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := identity.AsKey()
	if pid, ok := l.activations[key]; ok {
		return pid
	}
	if l.cluster == nil {
		return nil
	}

	kind, ok := l.cluster.TryGetClusterKind(identity.Kind)
	if !ok {
		return nil
	}

	props := cluster.WithClusterIdentity(kind.Props, identity)
	pid, err := l.cluster.ActorSystem.Root.SpawnNamed(props, activationName(identity))
	if err != nil {
		return nil
	}

	l.activations[key] = pid
	return pid
}

func (l *localIdentityLookup) RemovePid(identity *cluster.ClusterIdentity, pid *actor.PID) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := identity.AsKey()
	if existing, ok := l.activations[key]; ok && samePID(existing, pid) {
		delete(l.activations, key)
	}
}

func (l *localIdentityLookup) Setup(virtualCluster *cluster.Cluster, kinds []string, isClient bool) {
	l.cluster = virtualCluster
	l.terminatingSub = virtualCluster.ActorSystem.EventStream.Subscribe(func(evt interface{}) {
		switch msg := evt.(type) {
		case *cluster.ActivationTerminating:
			if msg.ClusterIdentity != nil {
				l.RemovePid(msg.ClusterIdentity, msg.Pid)
			}
		}
	})
}

func (l *localIdentityLookup) Shutdown() {
	if l.cluster != nil && l.terminatingSub != nil {
		l.cluster.ActorSystem.EventStream.Unsubscribe(l.terminatingSub)
		l.terminatingSub = nil
	}
}

func activationName(identity *cluster.ClusterIdentity) string {
	return fmt.Sprintf("%s-%s", identity.Kind, identity.Identity)
}

func samePID(left *actor.PID, right *actor.PID) bool {
	if left == nil || right == nil {
		return false
	}
	return left.Address == right.Address && left.Id == right.Id
}
