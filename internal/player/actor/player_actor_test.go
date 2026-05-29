package actor

import (
	"testing"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
	playerservice "overmind/internal/player/service"
)

type fakeLoginService struct{}

func (fakeLoginService) Login(playerID int64, worldID int64, account string) (playerservice.LoginResult, error) {
	return playerservice.LoginResult{
		PlayerID: playerID,
		WorldID:  worldID,
		Account:  account,
	}, nil
}

type immediateDataManager struct{}

func (immediateDataManager) Init(system *protoactor.ActorSystem, self *protoactor.PID) {
	system.Root.Send(self, playerInitialized{})
}

func (immediateDataManager) Tick() {}

func (immediateDataManager) Flush() bool { return true }

func TestPlayerActorRebindsChannelAndExpiresOldOne(t *testing.T) {
	system := protoactor.NewActorSystem()
	expired := make(chan string, 1)

	channelProps := protoactor.PropsFromFunc(func(ctx protoactor.Context) {
		switch msg := ctx.Message().(type) {
		case clustermsg.ChannelExpired:
			expired <- msg.ConnID
		}
	})
	oldChannel := system.Root.Spawn(channelProps)
	newChannel := system.Root.Spawn(channelProps)

	pid := system.Root.Spawn(Props(
		1001,
		fakeLoginService{},
		WithManagerFactory(func(playerID int64) DataManager { return immediateDataManager{} }),
		WithIdleTimeout(5*time.Second),
	))

	first, err := system.Root.RequestFuture(pid, clustermsg.PlayerLoginReq{
		PlayerID:   1001,
		WorldID:    1,
		Account:    "demo",
		ConnID:     "old",
		ChannelPID: oldChannel,
	}, time.Second).Result()
	if err != nil {
		t.Fatalf("first login failed: %v", err)
	}
	if resp, ok := first.(clustermsg.PlayerLoginResp); !ok || resp.ConnID != "old" {
		t.Fatalf("expected first login response for old conn, got %#v", first)
	}

	second, err := system.Root.RequestFuture(pid, clustermsg.PlayerLoginReq{
		PlayerID:   1001,
		WorldID:    1,
		Account:    "demo",
		ConnID:     "new",
		ChannelPID: newChannel,
	}, time.Second).Result()
	if err != nil {
		t.Fatalf("second login failed: %v", err)
	}
	if resp, ok := second.(clustermsg.PlayerLoginResp); !ok || resp.ConnID != "new" {
		t.Fatalf("expected second login response for new conn, got %#v", second)
	}

	select {
	case connID := <-expired:
		if connID != "old" {
			t.Fatalf("expected old conn to expire, got %q", connID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected old connection to expire")
	}
}

func TestPlayerActorPassivatesAfterChannelDisconnectAndIdleTimeout(t *testing.T) {
	system := protoactor.NewActorSystem()
	passivated := make(chan int64, 1)

	channelPID := system.Root.Spawn(protoactor.PropsFromFunc(func(ctx protoactor.Context) {}))
	pid := system.Root.Spawn(Props(
		1001,
		fakeLoginService{},
		WithManagerFactory(func(playerID int64) DataManager { return immediateDataManager{} }),
		WithIdleTimeout(50*time.Millisecond),
		WithTickInterval(10*time.Millisecond),
		WithOnPassivated(func(playerID int64) {
			passivated <- playerID
		}),
	))

	result, err := system.Root.RequestFuture(pid, clustermsg.PlayerLoginReq{
		PlayerID:   1001,
		WorldID:    1,
		Account:    "demo",
		ConnID:     "conn-1",
		ChannelPID: channelPID,
	}, time.Second).Result()
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if _, ok := result.(clustermsg.PlayerLoginResp); !ok {
		t.Fatalf("expected login response, got %#v", result)
	}

	system.Root.Stop(channelPID)

	select {
	case playerID := <-passivated:
		if playerID != 1001 {
			t.Fatalf("expected player 1001 to passivate, got %d", playerID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected player actor to passivate after channel disconnect")
	}
}
