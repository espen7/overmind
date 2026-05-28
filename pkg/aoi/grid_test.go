package aoi

import "testing"

func TestMoveReturnsPlayersFromCurrentAndNeighborCells(t *testing.T) {
	grid := NewGrid(1000, 1000, 100)
	grid.Upsert(1, 100, 100)
	grid.Upsert(2, 180, 180)
	grid.Upsert(3, 950, 950)

	visible := grid.VisibleTo(1)
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible entity, got %d", len(visible))
	}
	if visible[0] != 2 {
		t.Fatalf("expected player 2 to be visible, got %v", visible)
	}
}
