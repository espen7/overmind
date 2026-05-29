package data

import (
	"context"

	"overmind/internal/platform/persistence"
	playerdomain "overmind/internal/player/domain"
	playerrepo "overmind/internal/player/repository"
)

// PlayerActionMem 对齐 antares-main 的 action/timer/version 文档思路。
// 当前先把加载、增量 trace 和落库骨架补上，具体业务 action 后续再挂进来。
type PlayerActionMem struct {
	playerID int64
	repo     playerrepo.PlayerActionRepository
	tracker  *persistence.EntityMem[string, playerdomain.PlayerAction]

	maxActionID     int64
	actions         map[string]playerdomain.PlayerAction
	actionsByAction map[int32]playerdomain.PlayerAction
}

func NewPlayerActionMem(playerID int64, repo playerrepo.PlayerActionRepository) *PlayerActionMem {
	mem := &PlayerActionMem{
		playerID:        playerID,
		repo:            repo,
		actions:         make(map[string]playerdomain.PlayerAction),
		actionsByAction: make(map[int32]playerdomain.PlayerAction),
	}
	mem.tracker = persistence.NewEntityMem[string, playerdomain.PlayerAction](repo, mem.entities)
	return mem
}

func (m *PlayerActionMem) Init(ctx context.Context) error {
	actions, err := m.repo.LoadByPlayerID(ctx, m.playerID)
	if err != nil {
		return err
	}

	for _, action := range actions {
		m.actions[action.ID] = action
		m.actionsByAction[action.ActionID] = action
		if action.NumericID > m.maxActionID {
			m.maxActionID = action.NumericID
		}
	}
	return nil
}

func (m *PlayerActionMem) MarkClean() error {
	return m.tracker.MarkClean()
}

func (m *PlayerActionMem) TraceEntities() error {
	return m.tracker.TraceEntities()
}

func (m *PlayerActionMem) Flush(ctx context.Context) error {
	return m.tracker.Flush(ctx)
}

func (m *PlayerActionMem) GetOrCreateAction(actionID int32) playerdomain.PlayerAction {
	if action, ok := m.actionsByAction[actionID]; ok {
		return action
	}

	action := playerdomain.NewPlayerAction(m.playerID, m.maxActionID+1, actionID)
	m.maxActionID = action.NumericID
	m.actions[action.ID] = action
	m.actionsByAction[action.ActionID] = action
	return action
}

func (m *PlayerActionMem) entities() map[string]playerdomain.PlayerAction {
	return m.actions
}
