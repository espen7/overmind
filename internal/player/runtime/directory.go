package runtime

import (
	"fmt"
	"sync"

	protoactor "github.com/asynkron/protoactor-go/actor"

	playeractor "overmind/internal/player/actor"
	playerservice "overmind/internal/player/service"
)

// Directory 在当前 player 节点内维护 playerID -> PlayerActor 的唯一映射。
// 真正切到 cluster identity/sharding 前，先用它把“一玩家一个 actor”的语义跑通。
type Directory struct {
	root         *protoactor.RootContext
	loginService playerservice.LoginService
	options      []playeractor.Option

	mu     sync.Mutex
	actors map[int64]*protoactor.PID
}

func NewDirectory(root *protoactor.RootContext, loginService playerservice.LoginService, opts ...playeractor.Option) *Directory {
	return &Directory{
		root:         root,
		loginService: loginService,
		options:      opts,
		actors:       make(map[int64]*protoactor.PID),
	}
}

func (d *Directory) Resolve(playerID int64) *protoactor.PID {
	d.mu.Lock()
	defer d.mu.Unlock()

	if pid, ok := d.actors[playerID]; ok {
		return pid
	}

	opts := append([]playeractor.Option{}, d.options...)
	opts = append(opts, playeractor.WithOnPassivated(func(id int64) {
		d.remove(id)
	}))

	pid, err := d.root.SpawnNamed(
		playeractor.Props(playerID, d.loginService, opts...),
		fmt.Sprintf("player-%d", playerID),
	)
	if err != nil {
		panic(err)
	}

	d.actors[playerID] = pid
	return pid
}

func (d *Directory) remove(playerID int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.actors, playerID)
}

func (d *Directory) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.actors)
}
