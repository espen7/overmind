package mongo

import (
	"context"
	"fmt"
	"time"

	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"overmind/internal/platform/app"
)

// Connect 创建 Mongo client，并在启动阶段先做一次 ping，
// 这样 player 节点可以在依赖缺失时直接失败退出，而不是带着半残状态启动。
func Connect(ctx context.Context, cfg app.MongoConfig) (*mongodrv.Client, *mongodrv.Database, error) {
	client, err := mongodrv.Connect(options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, nil, fmt.Errorf("connect mongo: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("ping mongo: %w", err)
	}

	return client, client.Database(cfg.Database), nil
}
