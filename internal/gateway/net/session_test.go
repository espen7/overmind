package net

import "testing"

func TestSessionBindStoresPlayerAndToken(t *testing.T) {
	session := NewSession("conn-1")
	session.Bind(10001, "token-1")
	if session.PlayerID() != 10001 {
		t.Fatalf("expected bound player id, got %d", session.PlayerID())
	}
	if session.Token() != "token-1" {
		t.Fatalf("expected token to be preserved, got %q", session.Token())
	}
}
