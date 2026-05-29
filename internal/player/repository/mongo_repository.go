package repository

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"overmind/internal/player/domain"
)

var ErrPlayerNotFound = errors.New("player not found")

type PlayerRepository interface {
	Load(ctx context.Context, playerID int64) (domain.Player, error)
	UpsertMany(ctx context.Context, entities map[int64]domain.Player) error
	DeleteMany(ctx context.Context, ids []int64) error
}

type PlayerActionRepository interface {
	LoadByPlayerID(ctx context.Context, playerID int64) ([]domain.PlayerAction, error)
	UpsertMany(ctx context.Context, entities map[string]domain.PlayerAction) error
	DeleteMany(ctx context.Context, ids []string) error
}

type MongoRepository struct {
	playerCollection       *mongodrv.Collection
	playerActionCollection *mongodrv.Collection
}

func NewMongoRepository(database *mongodrv.Database) *MongoRepository {
	return &MongoRepository{
		playerCollection:       database.Collection("player"),
		playerActionCollection: database.Collection("player_action"),
	}
}

func (r *MongoRepository) Load(ctx context.Context, playerID int64) (domain.Player, error) {
	var player domain.Player
	err := r.playerCollection.FindOne(ctx, bson.M{"_id": playerID}).Decode(&player)
	if err == nil {
		return player, nil
	}
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return domain.Player{}, ErrPlayerNotFound
	}
	return domain.Player{}, fmt.Errorf("load player %d: %w", playerID, err)
}

func (r *MongoRepository) UpsertMany(ctx context.Context, entities map[int64]domain.Player) error {
	if len(entities) == 0 {
		return nil
	}

	models := make([]mongodrv.WriteModel, 0, len(entities))
	for _, entity := range entities {
		models = append(models, mongodrv.NewReplaceOneModel().
			SetFilter(bson.M{"_id": entity.ID}).
			SetReplacement(entity).
			SetUpsert(true),
		)
	}

	if _, err := r.playerCollection.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false)); err != nil {
		return fmt.Errorf("upsert players: %w", err)
	}
	return nil
}

func (r *MongoRepository) DeleteMany(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	if _, err := r.playerCollection.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}}); err != nil {
		return fmt.Errorf("delete players: %w", err)
	}
	return nil
}

func (r *MongoRepository) LoadByPlayerID(ctx context.Context, playerID int64) ([]domain.PlayerAction, error) {
	cursor, err := r.playerActionCollection.Find(ctx, bson.M{"player_id": playerID})
	if err != nil {
		return nil, fmt.Errorf("load player actions %d: %w", playerID, err)
	}
	defer cursor.Close(ctx)

	var actions []domain.PlayerAction
	if err := cursor.All(ctx, &actions); err != nil {
		return nil, fmt.Errorf("decode player actions %d: %w", playerID, err)
	}
	return actions, nil
}

func (r *MongoRepository) UpsertManyPlayerAction(ctx context.Context, entities map[string]domain.PlayerAction) error {
	if len(entities) == 0 {
		return nil
	}

	models := make([]mongodrv.WriteModel, 0, len(entities))
	for _, entity := range entities {
		models = append(models, mongodrv.NewReplaceOneModel().
			SetFilter(bson.M{"_id": entity.ID}).
			SetReplacement(entity).
			SetUpsert(true),
		)
	}

	if _, err := r.playerActionCollection.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false)); err != nil {
		return fmt.Errorf("upsert player actions: %w", err)
	}
	return nil
}

func (r *MongoRepository) DeleteManyPlayerAction(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	if _, err := r.playerActionCollection.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}}); err != nil {
		return fmt.Errorf("delete player actions: %w", err)
	}
	return nil
}

var _ PlayerActionRepository = (*playerActionRepoAdapter)(nil)

type playerActionRepoAdapter struct {
	repo *MongoRepository
}

func NewPlayerActionRepository(repo *MongoRepository) PlayerActionRepository {
	return &playerActionRepoAdapter{repo: repo}
}

func (a *playerActionRepoAdapter) LoadByPlayerID(ctx context.Context, playerID int64) ([]domain.PlayerAction, error) {
	return a.repo.LoadByPlayerID(ctx, playerID)
}

func (a *playerActionRepoAdapter) UpsertMany(ctx context.Context, entities map[string]domain.PlayerAction) error {
	return a.repo.UpsertManyPlayerAction(ctx, entities)
}

func (a *playerActionRepoAdapter) DeleteMany(ctx context.Context, ids []string) error {
	return a.repo.DeleteManyPlayerAction(ctx, ids)
}

var _ PlayerRepository = (*MongoRepository)(nil)

func (r *MongoRepository) Save(ctx context.Context, player domain.Player) error {
	if err := r.UpsertMany(ctx, map[int64]domain.Player{player.ID: player}); err != nil {
		return err
	}
	return nil
}
