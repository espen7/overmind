package actor

import (
	"context"
	"errors"
	"sync"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	playerdomain "overmind/internal/player/domain"
	playerrepo "overmind/internal/player/repository"
	playerservice "overmind/internal/player/service"
)

type mongoDataManager struct {
	playerID   int64
	repository playerrepo.PlayerRepository

	mu     sync.Mutex
	player playerdomain.Player
	loaded bool
	dirty  bool
}

func NewMongoManagerFactory(repository playerrepo.PlayerRepository) ManagerFactory {
	return func(playerID int64) DataManager {
		return &mongoDataManager{
			playerID:   playerID,
			repository: repository,
		}
	}
}

func (m *mongoDataManager) Init(system *protoactor.ActorSystem, self *protoactor.PID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		player, err := m.repository.Load(ctx, m.playerID)
		if err != nil {
			if !errors.Is(err, playerrepo.ErrPlayerNotFound) {
				system.Root.Send(self, playerInitializationFailed{Reason: err.Error()})
				return
			}

			player = playerdomain.NewPlayer(m.playerID)
			if err := m.repository.Save(ctx, player); err != nil {
				system.Root.Send(self, playerInitializationFailed{Reason: err.Error()})
				return
			}
		}

		m.mu.Lock()
		m.player = player
		m.loaded = true
		m.dirty = false
		m.mu.Unlock()

		system.Root.Send(self, playerInitialized{})
	}()
}

func (m *mongoDataManager) Tick() {
	if !m.isDirty() {
		return
	}
	_ = m.flush()
}

func (m *mongoDataManager) Flush() bool {
	if !m.isDirty() {
		return true
	}
	return m.flush() == nil
}

func (m *mongoDataManager) OnLogin(login playerservice.LoginResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.loaded {
		return
	}

	m.player.ApplyLogin(login.WorldID, login.Account)
	m.dirty = true
}

func (m *mongoDataManager) isDirty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loaded && m.dirty
}

func (m *mongoDataManager) flush() error {
	m.mu.Lock()
	if !m.loaded || !m.dirty {
		m.mu.Unlock()
		return nil
	}
	player := m.player
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.repository.Save(ctx, player); err != nil {
		return err
	}

	m.mu.Lock()
	if m.player.Version == player.Version {
		m.dirty = false
	}
	m.mu.Unlock()

	return nil
}
