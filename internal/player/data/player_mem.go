package data

import (
	"context"

	"overmind/internal/platform/persistence"
	playerdomain "overmind/internal/player/domain"
	playerrepo "overmind/internal/player/repository"
)

// PlayerMem 承载玩家主聚合。
// 第一版保持为单实体，但仍然走实体级 trace/flush，后面扩字段时不用再换框架。
type PlayerMem struct {
	playerID int64
	repo     playerrepo.PlayerRepository
	tracker  *persistence.EntityMem[int64, playerdomain.Player]

	player playerdomain.Player
}

func NewPlayerMem(playerID int64, repo playerrepo.PlayerRepository) *PlayerMem {
	mem := &PlayerMem{
		playerID: playerID,
		repo:     repo,
	}
	mem.tracker = persistence.NewEntityMem[int64, playerdomain.Player](repo, mem.entities)
	return mem
}

func (m *PlayerMem) Init(ctx context.Context) error {
	player, err := m.repo.Load(ctx, m.playerID)
	if err != nil {
		if err != playerrepo.ErrPlayerNotFound {
			return err
		}

		player = playerdomain.NewPlayer(m.playerID)
		if err := m.repo.UpsertMany(ctx, map[int64]playerdomain.Player{player.ID: player}); err != nil {
			return err
		}
	}

	m.player = player
	return nil
}

func (m *PlayerMem) MarkClean() error {
	return m.tracker.MarkClean()
}

func (m *PlayerMem) TraceEntities() error {
	return m.tracker.TraceEntities()
}

func (m *PlayerMem) Flush(ctx context.Context) error {
	return m.tracker.Flush(ctx)
}

func (m *PlayerMem) Player() *playerdomain.Player {
	return &m.player
}

func (m *PlayerMem) entities() map[int64]playerdomain.Player {
	if m.player.ID == 0 {
		return map[int64]playerdomain.Player{}
	}
	return map[int64]playerdomain.Player{m.player.ID: m.player}
}
