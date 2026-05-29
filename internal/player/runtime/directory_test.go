package runtime

import (
	"testing"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
	playeractor "overmind/internal/player/actor"
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

func TestDirectoryReusesPlayerActorAndRespawnsAfterPassivation(t *testing.T) {
	system := protoactor.NewActorSystem()
	directory := NewDirectory(
		system.Root,
		fakeLoginService{},
		playeractor.WithIdleTimeout(50*time.Millisecond),
		playeractor.WithTickInterval(10*time.Millisecond),
	)

	first := directory.Resolve(1001)
	second := directory.Resolve(1001)
	if first.GetId() != second.GetId() {
		t.Fatalf("expected same actor pid, first=%s second=%s", first, second)
	}
	if directory.Count() != 1 {
		t.Fatalf("expected one tracked player actor, got %d", directory.Count())
	}

	channelPID := system.Root.Spawn(protoactor.PropsFromFunc(func(ctx protoactor.Context) {}))
	result, err := system.Root.RequestFuture(first, clustermsg.PlayerLoginReq{
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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if directory.Count() == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if directory.Count() != 0 {
		t.Fatalf("expected directory to drop passivated actor, got %d entries", directory.Count())
	}

	third := directory.Resolve(1001)
	if third == nil {
		t.Fatal("expected directory to resolve player actor after passivation")
	}
	if directory.Count() != 1 {
		t.Fatalf("expected respawned actor to be tracked again, got %d entries", directory.Count())
	}
}
