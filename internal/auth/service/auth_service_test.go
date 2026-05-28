package service

import (
	"testing"

	"overmind/internal/auth/repository"
)

func TestLoginReturnsTokenAndPlayer(t *testing.T) {
	svc := New(repository.NewMemoryRepository())
	resp, err := svc.Login("demo", "demo")
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected token to be populated")
	}
	if resp.PlayerID == 0 {
		t.Fatal("expected player id to be populated")
	}
}

func TestValidateRejectsUnknownToken(t *testing.T) {
	svc := New(repository.NewMemoryRepository())
	if _, err := svc.Validate("missing"); err == nil {
		t.Fatal("expected validate to fail for unknown token")
	}
}
