package service

import "overmind/internal/world/domain"

type sceneRepository interface {
	UpsertPlayer(player domain.Player) domain.Player
	VisiblePlayers(sceneID int64, playerID int64) []domain.Player
	Monsters(sceneID int64) []domain.Monster
	SeedMonster(sceneID int64, id int64, name string, hp int32, maxHP int32, x int32, y int32)
	Player(id int64) (domain.Player, error)
}

type SceneService struct {
	world sceneRepository
}

func NewScene(world sceneRepository) *SceneService {
	return &SceneService{world: world}
}

// Enter creates or refreshes the player projection inside the world scene.
func (s *SceneService) Enter(playerID int64, name string, sceneID int64, x int32, y int32) (domain.EnterResult, error) {
	player := s.world.UpsertPlayer(domain.Player{
		ID:      playerID,
		Name:    name,
		SceneID: sceneID,
		X:       x,
		Y:       y,
	})

	if len(s.world.Monsters(sceneID)) == 0 {
		s.world.SeedMonster(sceneID, 9001, "Slime", 15, 15, 140, 120)
	}

	return domain.EnterResult{
		SceneID:         sceneID,
		Self:            player,
		VisiblePlayers:  s.world.VisiblePlayers(sceneID, playerID),
		VisibleMonsters: s.world.Monsters(sceneID),
	}, nil
}

// Move updates the player position and recomputes the nearby visibility list.
func (s *SceneService) Move(playerID int64, x int32, y int32) (domain.MoveResult, error) {
	player, err := s.world.Player(playerID)
	if err != nil {
		return domain.MoveResult{}, err
	}
	player.X = x
	player.Y = y
	player = s.world.UpsertPlayer(player)
	return domain.MoveResult{
		SceneID:        player.SceneID,
		Player:         player,
		VisiblePlayers: s.world.VisiblePlayers(player.SceneID, playerID),
	}, nil
}
