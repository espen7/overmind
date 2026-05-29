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
	Save(ctx context.Context, player domain.Player) error
}

type MongoRepository struct {
	collection *mongodrv.Collection
}

func NewMongoRepository(database *mongodrv.Database) *MongoRepository {
	return &MongoRepository{
		collection: database.Collection("player"),
	}
}

func (r *MongoRepository) Load(ctx context.Context, playerID int64) (domain.Player, error) {
	var player domain.Player
	err := r.collection.FindOne(ctx, bson.M{"_id": playerID}).Decode(&player)
	if err == nil {
		return player, nil
	}
	if errors.Is(err, mongodrv.ErrNoDocuments) {
		return domain.Player{}, ErrPlayerNotFound
	}
	return domain.Player{}, fmt.Errorf("load player %d: %w", playerID, err)
}

func (r *MongoRepository) Save(ctx context.Context, player domain.Player) error {
	_, err := r.collection.ReplaceOne(
		ctx,
		bson.M{"_id": player.ID},
		player,
		options.Replace().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("save player %d: %w", player.ID, err)
	}
	return nil
}
