package service

import (
	"testing"

	"overmind/internal/world/repository"
)

func TestEnterSceneReturnsSnapshotAndVisibleEntities(t *testing.T) {
	world := repository.NewMemoryWorld()
	world.SeedPlayer(10002, "Other", 1, 130, 130)

	svc := NewScene(world)
	result, err := svc.Enter(10001, "Hero", 1, 120, 120)
	if err != nil {
		t.Fatalf("enter returned error: %v", err)
	}
	if result.SceneID != 1 {
		t.Fatalf("unexpected scene id: %d", result.SceneID)
	}
	if len(result.VisiblePlayers) != 1 {
		t.Fatalf("expected one visible player, got %d", len(result.VisiblePlayers))
	}
	if result.VisiblePlayers[0].ID != 10002 {
		t.Fatalf("expected to see player 10002, got %+v", result.VisiblePlayers)
	}
}
