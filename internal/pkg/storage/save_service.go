package storage

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SaveCommand 表示一次异步落地请求
type SaveCommand struct {
	PlayerID    string
	Epoch       int64                  // 所有权任期号，落盘 filter 携带此值实现写入围栏 (fencing)
	DirtyFields map[string]interface{} // 字段级差量: "base.gold" -> 400, "bag.items" -> map[...]
	DoneCh      chan error             // 非 nil 时调用方阻塞等待写入结果（用于 Terminate 同步落地）
	Attempts    int                    // 已失败重试次数 (回投退避用, 调用方无需设置)
}

// 失败重试参数: BulkWrite 失败的批次回投队列指数退避重试。
// epoch 围栏保证重试永远安全: 迟到的重试要么落在自己 epoch 上成功,
// 要么被新主人抢占后的围栏作废, 不可能写脏
const (
	maxSaveAttempts = 10               // 累计重试上限, 超过后丢弃并告警
	maxSaveBackoff  = 30 * time.Second // 退避封顶
)

// SaveQueue 全局存盘队列，所有 PlayerActor 向此 channel 投递落地请求
var SaveQueue chan SaveCommand

var (
	saveWg      sync.WaitGroup
	saveClosing atomic.Bool // 关停标记: 置位后不再回投重试 (防止向已关闭队列投递)
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
	saveClosing.Store(true)
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

// flushBatch 批量写入 MongoDB（字段级 $set + epoch 写入围栏）
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

		// filter 携带 epoch：若文档已被新主人抢占 (epoch 已递增)，
		// 本次写入匹配不到任何文档而静默作废，防止旧主脏写。
		// 注意：不可 upsert，否则被围栏拒绝的写入会退化成插入新文档。
		model := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": cmd.PlayerID, "epoch": cmd.Epoch}).
			SetUpdate(bson.M{"$set": update})
		models = append(models, model)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.BulkWrite().SetOrdered(false)
	result, err := PlayerCol.BulkWrite(ctx, models, opts)

	if err != nil {
		log.Printf("[SaveService] BulkWrite 失败 (batch size: %d), 回投重试: %v", len(batch), err)
		requeueFailed(batch)
	} else if result.MatchedCount < int64(len(models)) {
		// 存在被 epoch 围栏拒绝的写入：说明有旧主人在所有权转移后仍尝试落盘，
		// 数据安全已由围栏保障，此处仅告警便于排查。
		log.Printf("[SaveService] 警告: %d 条写入被 epoch 围栏拒绝 (旧主人迟到写入已作废)",
			int64(len(models))-result.MatchedCount)
	}

	// 通知所有等待者 (同步等待者立即拿到本次结果, 不陪同重试阻塞——上游有自己的超时链)
	for _, cmd := range batch {
		if cmd.DoneCh != nil {
			cmd.DoneCh <- err
		}
	}
}

// requeueFailed 失败批次回投: 退避后重新入队重试, 超过上限丢弃并告警。
// 同步命令也转为异步回投 (调用方已拿到错误): 纯卸载场景下 Mongo 恢复后数据照样落地,
// 迁移场景下若新主已抢占则被围栏正确作废
func requeueFailed(batch []SaveCommand) {
	for _, cmd := range batch {
		cmd.Attempts++
		if cmd.Attempts >= maxSaveAttempts {
			log.Printf("[SaveService] 严重: 玩家 %s 落盘重试 %d 次仍失败, 该批增量丢弃 (脏字段数: %d)",
				cmd.PlayerID, cmd.Attempts, len(cmd.DirtyFields))
			continue
		}
		delay := time.Duration(1<<uint(cmd.Attempts-1)) * time.Second
		if delay > maxSaveBackoff {
			delay = maxSaveBackoff
		}
		c := cmd
		c.DoneCh = nil // 重试一律转异步
		go func() {
			defer func() {
				// 退避期间队列被关闭: 捕获投递 panic, 丢弃并告警
				if r := recover(); r != nil {
					log.Printf("[SaveService] 严重: 关停期间重试投递失败, 玩家 %s 增量丢弃", c.PlayerID)
				}
			}()
			time.Sleep(delay)
			if saveClosing.Load() {
				log.Printf("[SaveService] 严重: 服务关停中放弃重试, 玩家 %s 增量丢弃", c.PlayerID)
				return
			}
			SaveQueue <- c
		}()
	}
}
