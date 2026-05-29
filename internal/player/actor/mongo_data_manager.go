package actor

import (
	"context"
	"time"

	protoactor "github.com/asynkron/protoactor-go/actor"

	"overmind/internal/platform/logging"
	"overmind/internal/platform/persistence"
	playerdata "overmind/internal/player/data"
	playerrepo "overmind/internal/player/repository"
	playerservice "overmind/internal/player/service"
)

type mongoDataManager struct {
	playerID int64

	playerMem       *playerdata.PlayerMem
	playerActionMem *playerdata.PlayerActionMem

	mems       []persistence.MemData
	traceables []persistence.TraceableMemData
}

func NewMongoManagerFactory(
	playerRepository playerrepo.PlayerRepository,
	playerActionRepository playerrepo.PlayerActionRepository,
) ManagerFactory {
	return func(playerID int64) DataManager {
		playerMem := playerdata.NewPlayerMem(playerID, playerRepository)
		playerActionMem := playerdata.NewPlayerActionMem(playerID, playerActionRepository)

		return &mongoDataManager{
			playerID:        playerID,
			playerMem:       playerMem,
			playerActionMem: playerActionMem,
			mems: []persistence.MemData{
				playerMem,
				playerActionMem,
			},
			traceables: []persistence.TraceableMemData{
				playerMem,
				playerActionMem,
			},
		}
	}
}

func (m *mongoDataManager) Init(system *protoactor.ActorSystem, self *protoactor.PID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for _, mem := range m.mems {
			if err := mem.Init(ctx); err != nil {
				system.Root.Send(self, playerInitializationFailed{Reason: err.Error()})
				return
			}
		}

		for _, mem := range m.traceables {
			if err := mem.MarkClean(); err != nil {
				system.Root.Send(self, playerInitializationFailed{Reason: err.Error()})
				return
			}
		}

		system.Root.Send(self, playerInitialized{})
	}()
}

func (m *mongoDataManager) Tick() {
	for _, mem := range m.traceables {
		if err := mem.TraceEntities(); err != nil {
			logging.L().Warn(
				"player trace entities failed",
				logging.Int64("player_id", m.playerID),
				logging.Error(err),
			)
		}
	}
}

func (m *mongoDataManager) Flush() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok := true
	for _, mem := range m.traceables {
		if err := mem.Flush(ctx); err != nil {
			ok = false
			logging.L().Warn(
				"player flush failed",
				logging.Int64("player_id", m.playerID),
				logging.Error(err),
			)
		}
	}
	return ok
}

func (m *mongoDataManager) OnLogin(login playerservice.LoginResult) {
	player := m.playerMem.Player()
	if player == nil {
		return
	}
	player.ApplyLogin(login.WorldID, login.Account)
}
