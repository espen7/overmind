package storage

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"overmind/internal/pkg/routing"
)

// ringDocID home 哈希环文档的固定主键（单文档即整个集群的环真相源）
const ringDocID = "home_ring"

// RingDoc home 哈希环文档。集群唯一真相源，存于 cluster_meta 集合：
//   - 改环只走 ringctl 工具对本文档的 CAS 写（version 乐观锁，天然串行化并发修改）
//   - gate/home/world 三方周期轮询，version 变大即热切环，无中心进程、无推送依赖
//   - PrevNodes 为扩缩容过渡期的上一版环（协调器释放握手反查旧归属），commit 后清空
type RingDoc struct {
	ID        string   `bson:"_id"`
	Version   int      `bson:"version"`
	Nodes     []string `bson:"nodes"`
	PrevNodes []string `bson:"prev_nodes"`
}

// LoadRing 读取集群环文档，文档不存在时返回 mongo.ErrNoDocuments
func LoadRing() (*RingDoc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var doc RingDoc
	err := ClusterCol.FindOne(ctx, bson.M{"_id": ringDocID}).Decode(&doc)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// LoadOrSeedRing 读取集群环文档；文档缺失时用给定默认值原子播种（首次部署以 yaml 配置为准）。
// $setOnInsert + upsert 保证多节点并发首启时只有一个成功播种，其余读到同一份文档
func LoadOrSeedRing(defNodes []string, defVersion int, defPrevNodes []string) (*RingDoc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if defPrevNodes == nil {
		defPrevNodes = []string{}
	}
	update := bson.M{"$setOnInsert": bson.M{
		"version":    defVersion,
		"nodes":      defNodes,
		"prev_nodes": defPrevNodes,
	}}
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var doc RingDoc
	err := ClusterCol.FindOneAndUpdate(ctx, bson.M{"_id": ringDocID}, update, opts).Decode(&doc)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// CASUpdateRing 以乐观锁方式更新环文档（ringctl 工具专用）：
// 仅当文档当前版本等于 expectedVersion 时写入，否则返回冲突错误（调用方重读重试）。
// 新版本号必须由调用方置为 expectedVersion+1，保持版本单调递增
func CASUpdateRing(expectedVersion int, newDoc RingDoc) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if newDoc.Version != expectedVersion+1 {
		return fmt.Errorf("version_must_increase_by_one: expected %d, got %d", expectedVersion+1, newDoc.Version)
	}
	if newDoc.PrevNodes == nil {
		newDoc.PrevNodes = []string{}
	}

	filter := bson.M{"_id": ringDocID, "version": expectedVersion}
	update := bson.M{"$set": bson.M{
		"version":    newDoc.Version,
		"nodes":      newDoc.Nodes,
		"prev_nodes": newDoc.PrevNodes,
	}}
	res, err := ClusterCol.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("cas_conflict: 文档版本已不是 %d, 请重读后重试", expectedVersion)
	}
	return nil
}

// BuildRingManager 构建哈希环管理器: Mongo 已连接时以环文档为准 (文档缺失时用 yaml 配置播种),
// Mongo 未连接/读取失败时回落 yaml 静态环。两者都无节点时返回 nil (由调用方决定是否致命)。
// 返回的管理器后续应交给 StartRingPoller 持续热切
func BuildRingManager(defNodes []string, defVersion int, defPrevNodes []string, tag string) *routing.Manager {
	if ClusterCol != nil {
		var doc *RingDoc
		var err error
		if len(defNodes) > 0 {
			doc, err = LoadOrSeedRing(defNodes, defVersion, defPrevNodes)
		} else {
			// yaml 未配环时只读不播种, 避免向集群写入空环文档
			doc, err = LoadRing()
		}
		if err == nil {
			log.Printf("[Ring-%s] 已从 Mongo 环文档加载 Home 哈希环 (版本: %d, 节点: %v, 上一版: %v)",
				tag, doc.Version, doc.Nodes, doc.PrevNodes)
			return routing.NewManager(doc.Nodes, doc.Version, doc.PrevNodes)
		}
		log.Printf("[Ring-%s] 读取/播种环文档失败, 回落 yaml 静态环: %v", tag, err)
	}

	if len(defNodes) == 0 {
		return nil
	}
	log.Printf("[Ring-%s] 使用 yaml 配置构建 Home 哈希环 (版本: %d, 节点: %v)", tag, defVersion, defNodes)
	return routing.NewManager(defNodes, defVersion, defPrevNodes)
}

// StartRingPoller 启动环文档轮询协程：周期读取 Mongo 环文档，
// 版本变大时通过 Manager.Apply 热切环（版本单调保证乱序/重复读取无害）。
// Mongo 短暂不可达仅影响"感知新环"，不影响正在使用的旧环路由
func StartRingPoller(mgr *routing.Manager, interval time.Duration, tag string) {
	go func() {
		var lastErrMsg string
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			doc, err := LoadRing()
			if err != nil {
				// 同一错误只报一次，避免 Mongo 抖动期间日志刷屏
				if msg := err.Error(); msg != lastErrMsg {
					log.Printf("[RingPoller-%s] 读取环文档失败 (持旧环继续服务): %v", tag, err)
					lastErrMsg = msg
				}
				continue
			}
			lastErrMsg = ""

			if mgr.Apply(doc.Nodes, doc.Version, doc.PrevNodes) {
				log.Printf("[RingPoller-%s] 热切环成功: 版本 %d, 节点 %v, 上一版 %v",
					tag, doc.Version, doc.Nodes, doc.PrevNodes)
			}
		}
	}()
}
