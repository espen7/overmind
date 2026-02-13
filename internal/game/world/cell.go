package world

import (
	"log"

	"github.com/asynkron/protoactor-go/actor"
	kitpb "overmind/pkg/pb/kit"
)

// CellActor manages a spatial partition (Chunk) of the world.
// It handles:
// 1. Entities located within its bounds.
// 2. Movement logic collision checks (if any).
// 3. AoI (Area of Interest) notifications (broadcasting to nearby cells).
type CellActor struct {
	ChunkX int
	ChunkY int

	// In-memory entity storage
	// Key: EntityID, Value: Entity State (struct or protobuf)
	entities map[string]interface{}
}

func NewCellActor(chunkX, chunkY int) actor.Actor {
	return &CellActor{
		ChunkX:   chunkX,
		ChunkY:   chunkY,
		entities: make(map[string]interface{}),
	}
}

func (state *CellActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		// Load entities from DB persistence if needed
		log.Printf("Cell [%d,%d] started", state.ChunkX, state.ChunkY)

	case *kitpb.Envelope:
		// Handle Game Logic specific to this cell
		log.Printf("Cell [%d,%d] received envelope", state.ChunkX, state.ChunkY)

	case *actor.Stopping:
		log.Printf("Cell [%d,%d] stopping", state.ChunkX, state.ChunkY)
	}
}
