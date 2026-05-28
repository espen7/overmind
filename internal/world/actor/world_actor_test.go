package actor

import (
	"testing"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
	worldservice "overmind/internal/world/service"
)

type fakeWorldLoginService struct {
	nextID    int64
	playerIDs map[string]int64
}

func newFakeWorldLoginService() worldservice.LoginService {
	return &fakeWorldLoginService{
		nextID:    1000,
		playerIDs: make(map[string]int64),
	}
}

func (f *fakeWorldLoginService) ResolvePlayer(worldID int64, account string) (int64, bool, error) {
	if playerID, ok := f.playerIDs[account]; ok {
		return playerID, false, nil
	}
	f.nextID++
	f.playerIDs[account] = f.nextID
	return f.nextID, true, nil
}

func TestWorldActorCreatesAndReusesPlayerIdentity(t *testing.T) {
	system := protoactor.NewActorSystem()

	playerProps := protoactor.PropsFromFunc(func(ctx protoactor.Context) {
		switch msg := ctx.Message().(type) {
		case clustermsg.PlayerLoginReq:
			ctx.Respond(clustermsg.PlayerLoginResp{
				PlayerID: msg.PlayerID,
				WorldID:  msg.WorldID,
				ConnID:   msg.ConnID,
			})
		}
	})
	playerPID := system.Root.Spawn(playerProps)

	pid := system.Root.Spawn(Props(newFakeWorldLoginService(), func(playerID int64) *protoactor.PID {
		return playerPID
	}))

	firstResult, err := system.Root.RequestFuture(pid, clustermsg.WorldLoginReq{
		WorldID:    7,
		Account:    "demo",
		ConnID:     "conn-1",
		ChannelPID: playerPID,
	}, time.Second).Result()
	if err != nil {
		t.Fatalf("first login failed: %v", err)
	}
	firstResp, ok := firstResult.(clustermsg.PlayerLoginResp)
	if !ok {
		t.Fatalf("expected player login response, got %#v", firstResult)
	}
	if firstResp.PlayerID == 0 {
		t.Fatal("expected generated player id")
	}

	secondResult, err := system.Root.RequestFuture(pid, clustermsg.WorldLoginReq{
		WorldID:    7,
		Account:    "demo",
		ConnID:     "conn-2",
		ChannelPID: playerPID,
	}, time.Second).Result()
	if err != nil {
		t.Fatalf("second login failed: %v", err)
	}
	secondResp, ok := secondResult.(clustermsg.PlayerLoginResp)
	if !ok {
		t.Fatalf("expected player login response, got %#v", secondResult)
	}
	if secondResp.PlayerID != firstResp.PlayerID {
		t.Fatalf("expected player id reuse, first=%d second=%d", firstResp.PlayerID, secondResp.PlayerID)
	}
}
