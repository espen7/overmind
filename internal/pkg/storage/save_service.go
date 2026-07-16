package storage

import (
	"context"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SaveCommand 表示一次异步落地请求
type SaveCommand struct {
	PlayerID    string
	DirtyFields map[string]interface{} // 字段级差量: "base.gold" -> 400, "bag.items" -> map[...]
	DoneCh      chan error             // 非 nil 时调用方阻塞等待写入结果（用于 Terminate 同步落地）
}

// SaveQueue 全局存盘队列，所有 PlayerActor 向此 channel 投递落地请求
var SaveQueue chan SaveCommand

var (
	saveWg sync.WaitGroup
)

// InitSaveService 初始化全局异步存盘服务
// workerCount: 并发写入 worker 数量
// batchSize: 每批最多攒多少条再 flush
func InitSaveService(workerCount, batchSize int) {
	SaveQueue = make(chan SaveCommand, 4096)

	for i := 0; i < workerCount; i++ {
		saveWg.Add(1)
		go saveWorker(i, batchSize)
	}
	log.Printf("[SaveService] 启动成功，Worker 数量: %d，批量大小: %d", workerCount, batchSize)
}

// DrainAndClose 优雅关闭：关闭队列，等待所有 worker 排空并退出
func DrainAndClose() {
	close(SaveQueue) // 关闭 channel，worker 会自动退出循环
	saveWg.Wait()
	log.Printf("[SaveService] 所有 Worker 已退出，队列已排空")
}

// saveWorker 单个写入 worker，从队列中攒批执行 BulkWrite
func saveWorker(id int, batchSize int) {
	defer saveWg.Done()

	batch := make([]SaveCommand, 0, batchSize)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case cmd, ok := <-SaveQueue:
			if !ok {
				// channel 已关闭，flush 剩余并退出
				if len(batch) > 0 {
					flushBatch(batch)
				}
				return
			}
			batch = append(batch, cmd)
			if len(batch) >= batchSize {
				flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// 超时兜底，避免低流量时数据滞留
			if len(batch) > 0 {
				flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

// flushBatch 批量写入 MongoDB（字段级 $set）
func flushBatch(batch []SaveCommand) {
	if len(batch) == 0 {
		return
	}

	models := make([]mongo.WriteModel, 0, len(batch))

	for _, cmd := range batch {
		update := bson.M{}
		for fieldPath, value := range cmd.DirtyFields {
			update[fieldPath] = value
		}
		update["updated_at"] = time.Now()

		model := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": cmd.PlayerID}).
			SetUpdate(bson.M{"$set": update}).
			SetUpsert(true)
		models = append(models, model)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.BulkWrite().SetOrdered(false)
	_, err := PlayerCol.BulkWrite(ctx, models, opts)

	if err != nil {
		log.Printf("[SaveService] BulkWrite 失败 (batch size: %d): %v", len(batch), err)
	}

	// 通知所有等待者
	for _, cmd := range batch {
		if cmd.DoneCh != nil {
			cmd.DoneCh <- err
		}
	}
}
