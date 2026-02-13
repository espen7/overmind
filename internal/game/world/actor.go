package world

import (
	"fmt"
	"log"

	"github.com/asynkron/protoactor-go/actor"
	kitpb "overmind/pkg/pb/kit"
)

// WorldActor represents a conceptual "Space" or "Map Layer".
// It is responsible for:
// 1. Spawning and supervising CellActors (Chunks).
// 2. Routing messages to specific Cells based on coordinates.
// 3. Broadcasting global events.
type WorldActor struct {
	mapWidth  int
	mapHeight int
	cellSize  int

	cells map[string]*actor.PID // Key: "x_y" -> PID
}

func NewWorldActor() actor.Actor {
	return &WorldActor{
		mapWidth:  1000, // Example size
		mapHeight: 1000,
		cellSize:  50,
		cells:     make(map[string]*actor.PID),
	}
}

func (state *WorldActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		log.Println("WorldActor started")
		state.initializeCells(context)

	case *kitpb.Envelope:
		// Handle Map Logic
		// In BigWorld, we usually route by Position.
		// For simplicity, let's assume the message contains coordinate info or target EntityID.

		// Example: If it's a "MoveReq", calculate target Cell and forward.
		log.Printf("World received envelope trace: %s", msg.TraceId)

	case *actor.Stopping:
		log.Println("WorldActor stopping")
	}
}

func (state *WorldActor) initializeCells(ctx actor.Context) {
	// Lazy load or Pre-spawn?
	// Let's pre-spawn a few for demo.
	// Logic: Cell ID = X_Y (Chunk Coords)

	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			state.spawnCell(ctx, x, y)
		}
	}
}

func (state *WorldActor) spawnCell(ctx actor.Context, chunkX, chunkY int) {
	cellID := fmt.Sprintf("cell_%d_%d", chunkX, chunkY)
	props := actor.PropsFromProducer(func() actor.Actor {
		return NewCellActor(chunkX, chunkY)
	})

	pid, err := ctx.SpawnNamed(props, cellID)
	if err != nil {
		log.Printf("Failed to spawn cell %s: %v", cellID, err)
		return
	}

	state.cells[cellID] = pid
	log.Printf("Spawned Cell %s", cellID)
}
