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
	// 这里维护的是世界侧投影，不是玩家全量主数据。
	// 后续接 player/world 双节点后，world 仍然只关心地图运行所需的最小状态。
	player := s.world.UpsertPlayer(domain.Player{
		ID:      playerID,
		Name:    name,
		SceneID: sceneID,
		X:       x,
		Y:       y,
	})

	if len(s.world.Monsters(sceneID)) == 0 {
		// 第一版为了把场景链路跑通，首次进图时自动播一只默认怪。
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
	// 位置写回后立刻重算可见列表，保证 AOI 广播始终围绕最新坐标展开。
	return domain.MoveResult{
		SceneID:        player.SceneID,
		Player:         player,
		VisiblePlayers: s.world.VisiblePlayers(player.SceneID, playerID),
	}, nil
}
