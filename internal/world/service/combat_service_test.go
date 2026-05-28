package service

import (
	"testing"

	"overmind/internal/world/repository"
)

func TestAttackReducesMonsterHPAndMarksDeath(t *testing.T) {
	world := repository.NewMemoryWorld()
	world.SeedMonster(1, 9001, "Slime", 15, 8, 120, 120)

	svc := NewCombat(world)
	result, err := svc.Attack(10001, 1, 9001)
	if err != nil {
		t.Fatalf("attack returned error: %v", err)
	}
	if result.MonsterHP >= 15 {
		t.Fatalf("expected hp drop, got %d", result.MonsterHP)
	}
	if !result.Dead {
		t.Fatal("expected monster to be dead after attack")
	}
}
