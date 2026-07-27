package storage

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Client 全局 MongoDB 客户端
var Client *mongo.Client

// PlayerCol 玩家数据集合
var PlayerCol *mongo.Collection

// ClusterCol 集群元数据集合 (home 哈希环文档等)
var ClusterCol *mongo.Collection

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
	ClusterCol = client.Database(dbName).Collection("cluster_meta")
	return nil
}

// AcquireOwnership 抢占玩家文档所有权（epoch 围栏机制的核心）。
// 通过原子 $inc epoch 获取单调递增的任期号，后续落盘必须携带该 epoch；
// 旧主人（持有旧 epoch）的迟到写入因 filter 不匹配而自动作废，从而安全化一切双主/脑裂场景。
// upsert 保证新玩家文档预创建（仅含 _id/epoch/owner_node，业务数据由 PlayerModel 补齐）。
func AcquireOwnership(playerID string, ownerNode string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	update := bson.M{
		"$inc": bson.M{"epoch": int64(1)},
		"$set": bson.M{"owner_node": ownerNode},
	}
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After).
		SetProjection(bson.M{"epoch": 1})

	var doc struct {
		Epoch int64 `bson:"epoch"`
	}
	err := PlayerCol.FindOneAndUpdate(ctx, bson.M{"_id": playerID}, update, opts).Decode(&doc)
	if err != nil {
		return 0, err
	}
	return doc.Epoch, nil
}

// CloseDB 关闭 MongoDB 连接
func CloseDB() {
	if Client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = Client.Disconnect(ctx)
	}
}
