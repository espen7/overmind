package actor

import (
	"testing"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
)

func TestChannelActorRoutesLoginToWorld(t *testing.T) {
	system := protoactor.NewActorSystem()
	worldMessages := make(chan clustermsg.WorldLoginReq, 1)

	worldPID := system.Root.Spawn(protoactor.PropsFromFunc(func(ctx protoactor.Context) {
		switch msg := ctx.Message().(type) {
		case clustermsg.WorldLoginReq:
			worldMessages <- msg
			ctx.Respond(clustermsg.PlayerLoginResp{
				PlayerID: 1001,
				WorldID:  msg.WorldID,
				ConnID:   msg.ConnID,
			})
		}
	}))

	pid := system.Root.Spawn(Props("conn-1", worldPID, func(playerID int64) *protoactor.PID {
		return nil
	}))

	system.Root.Send(pid, LoginFrame{
		WorldID: 7,
		Account: "demo",
	})

	select {
	case msg := <-worldMessages:
		if msg.WorldID != 7 {
			t.Fatalf("expected world id 7, got %d", msg.WorldID)
		}
		if msg.Account != "demo" {
			t.Fatalf("expected account demo, got %q", msg.Account)
		}
		if msg.ConnID != "conn-1" {
			t.Fatalf("expected conn id conn-1, got %q", msg.ConnID)
		}
		if msg.ChannelPID == nil {
			t.Fatal("expected channel pid to be attached")
		}
	case <-time.After(time.Second):
		t.Fatal("expected login request to reach world actor")
	}
}

func TestChannelActorRoutesPlayerEnvelopeAfterAuthorization(t *testing.T) {
	system := protoactor.NewActorSystem()
	playerMessages := make(chan any, 1)
	playerPID := system.Root.Spawn(protoactor.PropsFromFunc(func(ctx protoactor.Context) {
		if _, ok := ctx.Message().(*protoactor.Started); ok {
			return
		}
		playerMessages <- ctx.Message()
	}))
	worldPID := system.Root.Spawn(protoactor.PropsFromFunc(func(ctx protoactor.Context) {}))

	pid := system.Root.Spawn(Props("conn-2", worldPID, func(playerID int64) *protoactor.PID {
		if playerID == 1001 {
			return playerPID
		}
		return nil
	}))

	system.Root.Send(pid, clustermsg.PlayerLoginResp{
		PlayerID: 1001,
		WorldID:  7,
		ConnID:   "conn-2",
	})
	system.Root.Send(pid, ClientPlayerEnvelope{Payload: "march"})

	select {
	case msg := <-playerMessages:
		if msg != "march" {
			t.Fatalf("expected player payload march, got %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected player envelope to reach player actor")
	}
}
