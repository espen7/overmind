package transport

import (
	"google.golang.org/protobuf/proto"

	"overmind/internal/game/repository"
	"overmind/internal/game/service"
	"overmind/internal/gateway/protocol"
	gamepb "overmind/pkg/pb/game"
)

type Outbound struct {
	Recipients []int64
	Type       uint16
	Message    proto.Message
}

type Handler struct {
	scene  *service.SceneService
	combat *service.CombatService
	world  *repository.MemoryWorld
}

func NewHandler(scene *service.SceneService, combat *service.CombatService, world *repository.MemoryWorld) *Handler {
	return &Handler{
		scene:  scene,
		combat: combat,
		world:  world,
	}
}

func (h *Handler) EnterScene(playerID int64, name string, sceneID int64, x int32, y int32) ([]Outbound, error) {
	result, err := h.scene.Enter(playerID, name, sceneID, x, y)
	if err != nil {
		return nil, err
	}
	return []Outbound{{
		Recipients: []int64{playerID},
		Type:       protocol.MessageTypeSceneSnapshot,
		Message:    service.BuildSnapshot(result),
	}}, nil
}

func (h *Handler) Move(playerID int64, req *gamepb.MoveRequest) ([]Outbound, error) {
	result, err := h.scene.Move(playerID, req.GetX(), req.GetY())
	if err != nil {
		return nil, err
	}
	recipients := []int64{playerID}
	for _, player := range result.VisiblePlayers {
		recipients = append(recipients, player.ID)
	}
	return []Outbound{{
		Recipients: recipients,
		Type:       protocol.MessageTypeMoveBroadcast,
		Message:    service.BuildMoveBroadcast(result),
	}}, nil
}

func (h *Handler) Attack(playerID int64, req *gamepb.AttackRequest) ([]Outbound, error) {
	player, err := h.world.Player(playerID)
	if err != nil {
		return nil, err
	}
	result, err := h.combat.Attack(playerID, player.SceneID, req.GetTargetId())
	if err != nil {
		return nil, err
	}
	recipients := []int64{playerID}
	for _, visible := range h.world.VisiblePlayers(player.SceneID, playerID) {
		recipients = append(recipients, visible.ID)
	}
	return []Outbound{{
		Recipients: recipients,
		Type:       protocol.MessageTypeCombatBroadcast,
		Message:    service.BuildCombatBroadcast(result),
	}}, nil
}
