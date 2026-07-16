package storage

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Client 全局 MongoDB 客户端
var Client *mongo.Client

// PlayerCol 玩家数据集合
var PlayerCol *mongo.Collection

// InitDB 初始化 MongoDB 连接，建立 players 集合引用
func InitDB(uri string, dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return err
	}

	// Ping 验证连接
	if err = client.Ping(ctx, nil); err != nil {
		return err
	}

	Client = client
	PlayerCol = client.Database(dbName).Collection("players")
	return nil
}

// CloseDB 关闭 MongoDB 连接
func CloseDB() {
	if Client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = Client.Disconnect(ctx)
	}
}
