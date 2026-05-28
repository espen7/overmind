package repository

import (
	"fmt"
	"sort"
	"sync"

	"overmind/internal/game/domain"
	"overmind/pkg/aoi"
)

type MemoryWorld struct {
	mu       sync.RWMutex
	players  map[int64]domain.Player
	monsters map[int64]map[int64]domain.Monster
	grids    map[int64]*aoi.Grid
}

func NewMemoryWorld() *MemoryWorld {
	return &MemoryWorld{
		players:  make(map[int64]domain.Player),
		monsters: make(map[int64]map[int64]domain.Monster),
		grids:    make(map[int64]*aoi.Grid),
	}
}

func (w *MemoryWorld) SeedPlayer(id int64, name string, sceneID int64, x int32, y int32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	player := domain.Player{ID: id, Name: name, SceneID: sceneID, X: x, Y: y}
	w.players[id] = player
	w.ensureGrid(sceneID).Upsert(id, x, y)
}

func (w *MemoryWorld) SeedMonster(sceneID int64, id int64, name string, hp int32, maxHP int32, x int32, y int32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ensureMonsters(sceneID)[id] = domain.Monster{
		ID:      id,
		Name:    name,
		SceneID: sceneID,
		HP:      hp,
		MaxHP:   maxHP,
		X:       x,
		Y:       y,
	}
}

func (w *MemoryWorld) UpsertPlayer(player domain.Player) domain.Player {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.players[player.ID] = player
	w.ensureGrid(player.SceneID).Upsert(player.ID, player.X, player.Y)
	return player
}

func (w *MemoryWorld) Player(id int64) (domain.Player, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	player, ok := w.players[id]
	if !ok {
		return domain.Player{}, fmt.Errorf("player not found")
	}
	return player, nil
}

func (w *MemoryWorld) VisiblePlayers(sceneID int64, playerID int64) []domain.Player {
	w.mu.RLock()
	defer w.mu.RUnlock()

	grid, ok := w.grids[sceneID]
	if !ok {
		return nil
	}
	ids := grid.VisibleTo(playerID)
	result := make([]domain.Player, 0, len(ids))
	for _, id := range ids {
		player, exists := w.players[id]
		if exists && player.SceneID == sceneID {
			result = append(result, player)
		}
	}
	sort.Slice(result, func(i int, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (w *MemoryWorld) Monsters(sceneID int64) []domain.Monster {
	w.mu.RLock()
	defer w.mu.RUnlock()

	monsters := w.monsters[sceneID]
	result := make([]domain.Monster, 0, len(monsters))
	for _, monster := range monsters {
		result = append(result, monster)
	}
	sort.Slice(result, func(i int, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (w *MemoryWorld) FindMonster(sceneID int64, targetID int64) (domain.Monster, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	monster, ok := w.monsters[sceneID][targetID]
	if !ok {
		return domain.Monster{}, fmt.Errorf("monster not found")
	}
	return monster, nil
}

func (w *MemoryWorld) SaveMonster(sceneID int64, monster domain.Monster) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ensureMonsters(sceneID)[monster.ID] = monster
}

func (w *MemoryWorld) ensureGrid(sceneID int64) *aoi.Grid {
	grid, ok := w.grids[sceneID]
	if !ok {
		grid = aoi.NewGrid(1000, 1000, 100)
		w.grids[sceneID] = grid
	}
	return grid
}

func (w *MemoryWorld) ensureMonsters(sceneID int64) map[int64]domain.Monster {
	monsters, ok := w.monsters[sceneID]
	if !ok {
		monsters = make(map[int64]domain.Monster)
		w.monsters[sceneID] = monsters
	}
	return monsters
}
