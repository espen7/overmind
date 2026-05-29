package actor

import (
	"testing"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	clustermsg "overmind/internal/cluster/messages"
	kitpb "overmind/pkg/pb/kit"
)

func TestChannelActorRoutesLoginToWorld(t *testing.T) {
	system := protoactor.NewActorSystem()
	worldMessages := make(chan *kitpb.WorldLoginRequest, 1)

	worldPID := system.Root.Spawn(protoactor.PropsFromFunc(func(ctx protoactor.Context) {
		switch msg := ctx.Message().(type) {
		case *kitpb.Envelope:
			request, err := clustermsg.DecodeWorldLoginEnvelope(msg)
			if err != nil {
				t.Fatalf("decode world login envelope: %v", err)
			}
			worldMessages <- request
			reply, err := clustermsg.NewWorldLoginResponseEnvelope(&kitpb.WorldLoginResponse{
				PlayerId: 1001,
				WorldId:  request.GetWorldId(),
				ConnId:   request.GetConnId(),
			})
			if err != nil {
				t.Fatalf("encode world login response: %v", err)
			}
			ctx.Respond(reply)
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
		if msg.GetWorldId() != 7 {
			t.Fatalf("expected world id 7, got %d", msg.GetWorldId())
		}
		if msg.GetAccount() != "demo" {
			t.Fatalf("expected account demo, got %q", msg.GetAccount())
		}
		if msg.GetConnId() != "conn-1" {
			t.Fatalf("expected conn id conn-1, got %q", msg.GetConnId())
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

	reply, err := clustermsg.NewWorldLoginResponseEnvelope(&kitpb.WorldLoginResponse{
		PlayerId: 1001,
		WorldId:  7,
		ConnId:   "conn-2",
	})
	if err != nil {
		t.Fatalf("encode world login response: %v", err)
	}
	system.Root.Send(pid, reply)
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
