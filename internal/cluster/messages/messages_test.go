package messages

import "testing"

func TestPlayerLoginReqTargetsPlayerIdentity(t *testing.T) {
	req := PlayerLoginReq{
		PlayerID: 1001,
		WorldID:  1,
		Account:  "demo",
		ConnID:   "conn-1",
	}

	if got := req.Identity(); got != "player/1001" {
		t.Fatalf("expected identity player/1001, got %q", got)
	}
}

func TestWorldLoginReqTargetsWorldIdentity(t *testing.T) {
	req := WorldLoginReq{
		WorldID: 7,
		Account: "demo",
		ConnID:  "conn-2",
	}

	if got := req.Identity(); got != "world/7" {
		t.Fatalf("expected identity world/7, got %q", got)
	}
}
