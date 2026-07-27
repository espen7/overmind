package home

import (
	"fmt"
	"log"
	"strings"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"google.golang.org/protobuf/proto"

	"overmind/api/game"
	"overmind/api/rpc"
	"overmind/internal/pkg/actor"
	"overmind/internal/pkg/network"
	"overmind/internal/pkg/routing"
	"overmind/internal/pkg/storage"
)

// releasePrefix 跨节点释放握手请求前缀: "release:<playerID>"
const releasePrefix = "release:"

// ==================== HomeCoordinatorActor ====================

// HomeCoordinatorActor 管理本地节点所有的 PlayerActor 实例
type HomeCoordinatorActor struct {
	act.Actor
	ringMgr   *routing.Manager // 哈希环管理器，用于释放握手时反查旧归属
	worldNode string           // World 节点名, 透传给 PlayerActor 寻址 world_actor
}

// Init 初始化协调器
func (hc *HomeCoordinatorActor) Init(args ...any) error {
	if len(args) >= 1 {
		if mgr, ok := args[0].(*routing.Manager); ok {
			hc.ringMgr = mgr
		}
	}
	if len(args) >= 2 {
		if node, ok := args[1].(string); ok {
			hc.worldNode = node
		}
	}
	log.Printf("[HomeCoordinatorActor] 协调器启动成功")
	return nil
}

// HandleCall 响应同步请求：
//   - "<playerID>": 分配/创建 PlayerActor (来自网关)
//   - "release:<playerID>": 释放本地 PlayerActor (来自新归属节点的释放握手)
//
// 注意: 失败一律以 "error: xxx" 字符串作为结果返回, 绝不能返回非 nil error——
// ergo 会把非 nil error 当作终止原因杀掉协调器本身 (无 supervisor, 死了不会重启)
func (hc *HomeCoordinatorActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// 使用 Go 原生基本类型 string 作为请求入参，避免序列化配置开销
	req, ok := request.(string)
	if !ok {
		return "error: invalid_request_type", nil
	}

	// 分支 1: 释放握手请求，要求本地旧 Actor 同步落盘并退位
	if strings.HasPrefix(req, releasePrefix) {
		return hc.handleRelease(strings.TrimPrefix(req, releasePrefix))
	}

	// 分支 2: 分配/创建 PlayerActor
	playerID := req

	// 所有权自检: 环切换过渡期内持旧环的调用方(网关/world)可能找错节点,
	// 若直接分配会误抢回所有权; 按投递契约返回明确错误, 由调用方按其当前环重试
	if hc.ringMgr != nil {
		if owner := hc.ringMgr.PickCurrent(playerID); owner != string(hc.Node().Name()) {
			log.Printf("[HomeCoordinatorActor] 拒绝分配 Player %s: 按当前环归属 %s 而非本节点", playerID, owner)
			return "error: wrong_owner", nil
		}
	}

	actorName := fmt.Sprintf("player_actor_%s", playerID)

	// 1. 检查是否已经创建过该玩家的 Actor（本节点已持有所有权，无需重新抢占）
	pid, err := hc.Node().ProcessPID(gen.Atom(actorName))
	if err == nil {
		log.Printf("[HomeCoordinatorActor] Player %s 已经在线/在内存缓冲中, 直接返回 PID: %s", playerID, pid.String())
		return pid, nil
	}

	// 2. 释放握手：若存在上一版环且旧归属不是自己，先请旧主节点落盘退位，
	// 保证本节点从 DB 读到的是完整数据（失败不阻断，epoch 围栏兜底）
	hc.releaseFromOldOwner(playerID)

	// 3. 抢占所有权：原子递增 epoch，从此旧主人的迟到写入全部作废
	epoch, err := storage.AcquireOwnership(playerID, string(hc.Node().Name()))
	if err != nil {
		log.Printf("[HomeCoordinatorActor] 抢占 Player %s 所有权失败: %v", playerID, err)
		return "error: acquire_ownership_failed", nil
	}

	// 4. 惰性加载：Spawn 创建新的 PlayerActor（携带任期号与 world 寻址）
	newPID, err := hc.SpawnRegister(gen.Atom(actorName), func() gen.ProcessBehavior {
		return &PlayerActor{}
	}, gen.ProcessOptions{}, playerID, epoch, hc.worldNode)

	if err != nil {
		log.Printf("[HomeCoordinatorActor] 启动 PlayerActor 失败: %v", err)
		return "error: spawn_failed", nil
	}

	log.Printf("[HomeCoordinatorActor] 成功为 Player %s 创建专属 PlayerActor: %s (epoch: %d)", playerID, newPID.String(), epoch)
	return newPID, nil
}

// releaseFromOldOwner 向旧归属节点发起释放握手（正常迁移路径，保障不丢增量）
func (hc *HomeCoordinatorActor) releaseFromOldOwner(playerID string) {
	if hc.ringMgr == nil {
		return
	}
	oldNode := hc.ringMgr.PickPrevious(playerID)
	if oldNode == "" || oldNode == string(hc.Node().Name()) {
		return
	}

	log.Printf("[HomeCoordinatorActor] Player %s 旧归属为 %s, 发起释放握手...", playerID, oldNode)
	oldCoordinator := gen.ProcessID{Name: gen.Atom("home_coordinator"), Node: gen.Atom(oldNode)}
	// 超时链对齐: 本层 10s 必须包住旧协调器内层对 PlayerActor 的 8s 等待,
	// 否则旧 Actor 清空积压邮箱较慢时, 本层先超时抢占 epoch, 旧主已消费的增量会被围栏作废
	res, err := hc.CallWithTimeout(oldCoordinator, releasePrefix+playerID, 10)
	if err != nil {
		// 旧节点宕机/超时：继续抢占，最坏丢失旧主最近一个异步周期的增量，
		// 脏写风险由 epoch 围栏拦截
		log.Printf("[HomeCoordinatorActor] 释放握手失败 (epoch 围栏兜底继续): %v", err)
	} else if res != "released" {
		// 旧协调器回了错误体 (如旧 Actor 退位失败)：同样继续抢占, 围栏兜底
		log.Printf("[HomeCoordinatorActor] 释放握手异常回执 (epoch 围栏兜底继续): %v", res)
	}
}

// handleRelease 处理释放请求：若本地持有该玩家 Actor，同步要求其落盘并退位
func (hc *HomeCoordinatorActor) handleRelease(playerID string) (any, error) {
	actorName := fmt.Sprintf("player_actor_%s", playerID)
	pid, err := hc.Node().ProcessPID(gen.Atom(actorName))
	if err != nil {
		// 本地没有该玩家 Actor，直接确认
		return "released", nil
	}

	// 同步 Call 目标 Actor: 其 HandleCall("release") 会阻塞完成全量落盘后才回包退出，
	// 因此本 Call 返回即代表数据已安全在 DB 中
	_, err = hc.CallWithTimeout(pid, "release", 8)
	if err != nil {
		log.Printf("[HomeCoordinatorActor] 要求 Player %s 退位失败: %v", playerID, err)
		return "error: release_failed", nil
	}

	log.Printf("[HomeCoordinatorActor] Player %s 已落盘退位, 释放握手完成", playerID)
	return "released", nil
}

// ==================== PlayerActor ====================

// PlayerActor 玩家个人逻辑状态机，生命周期与 Socket 网关 Actor Link 绑定
type PlayerActor struct {
	act.Actor
	playerID  string
	epoch     int64   // 所有权任期号，随每次落盘携带实现写入围栏
	worldNode string  // World 节点名, 寻址 world_actor
	gatePID   gen.PID // 绑定的网关 Channel Actor PID
	released  bool    // 已通过释放握手落盘退位，Terminate 无需重复落盘

	// 玩家数据模型（Actor 持有，Init 时从 DB 加载）
	model *PlayerModel

	// 客户端差量同步脏标记
	dirtyGold  bool
	dirtyPower bool

	// 卸载定时器的取消函数
	unloadCancel gen.CancelFunc
}

// Init 初始化玩家数据
func (pa *PlayerActor) Init(args ...any) error {
	if len(args) < 2 {
		return fmt.Errorf("bad_arguments")
	}

	playerID, ok1 := args[0].(string)
	epoch, ok2 := args[1].(int64)
	if !ok1 || !ok2 {
		return fmt.Errorf("bad_arguments")
	}

	pa.playerID = playerID
	pa.epoch = epoch
	if len(args) >= 3 {
		if node, ok := args[2].(string); ok {
			pa.worldNode = node
		}
	}

	// 1. 启用退出信号捕获，保证网关断开时自己不会立刻退出
	pa.SetTrapExit(true)

	// 2. 加载玩家数据（Model 封装了 DB 操作）
	pa.model = NewPlayerModel(playerID)
	isNew, err := pa.model.Load()
	if err != nil {
		log.Printf("[PlayerActor] 玩家 %s 数据加载失败: %v", playerID, err)
		return err
	}

	if isNew {
		log.Printf("[PlayerActor] 成功为新玩家 %s 创建初始数据!", pa.playerID)
	}

	log.Printf("[PlayerActor] 玩家 %s 状态机载入成功 (epoch: %d, Gold: %d, Power: %d, BuildLevel: %d)",
		pa.playerID, pa.epoch, pa.model.Profile.GetGold(), pa.model.Profile.GetPower(), pa.model.Profile.GetBuildLevel())

	// 3. 启动周期性的异步落盘定时器（每 5 秒触发一次）
	_, _ = pa.SendAfter(pa.PID(), "async_save_tick", 5*time.Second)

	return nil
}

// HandleMessage 处理网关/定时器转发来的跨进程普通消息
func (pa *PlayerActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case gen.MessageExitPID:
		// 1. 网关 ChannelActor 退出，进入下线缓冲状态
		log.Printf("[PlayerActor] Player %s 关联网关断开 (Link Exit), 启动 5 秒下线自动卸载定时器...", pa.playerID)
		pa.gatePID = gen.PID{} // 清除网关 PID

		// 派生 5 秒注销定时器
		cancel, err := pa.SendAfter(pa.PID(), "unload_timeout", 5*time.Second)
		if err == nil {
			pa.unloadCancel = cancel
		}
		return nil

	case string:
		if msg == "unload_timeout" {
			log.Printf("[PlayerActor] Player %s 5 秒超时未重连，执行自愈销毁 Actor...", pa.playerID)
			return gen.TerminateReasonNormal
		}

		if msg == "async_save_tick" {
			// 定时异步落盘
			pa.flushDirty()
			// 重启下一次定时异步落盘
			_, _ = pa.SendAfter(pa.PID(), "async_save_tick", 5*time.Second)
			return nil
		}

	case []byte:
		// 解包 RPCEnvelope 数据包 (来自网关转发或 world 节点内部投递)
		envelope := &rpc.RPCEnvelope{}
		if err := proto.Unmarshal(msg, envelope); err != nil {
			log.Printf("[PlayerActor] 解包 RPCEnvelope 错误: %v", err)
			return nil
		}

		// 内部协议段 (90000+): 来自 world 等后端节点, 不是网关, 不能触碰网关绑定状态
		if envelope.ProtoId >= 90000 {
			pa.handleInternalMessage(envelope)
			return nil
		}

		pa.gatePID = from

		// 收到新网关数据包，安全检查：如果卸载定时器活跃，撤销该定时器
		if pa.unloadCancel != nil {
			pa.unloadCancel()
			pa.unloadCancel = nil
			log.Printf("[PlayerActor] Player %s 收到新网关数据包，取消下线自动注销定时器", pa.playerID)
		}

		switch envelope.ProtoId {
		case int32(game.MsgID_MSG_C2S_BUILD_UPGRADE):
			pa.handleBuildUpgrade(envelope.Payload)
		}
	}

	return nil
}

// HandleCall 处理同步调用 (重连网关重新绑定 / 释放握手退位)
func (pa *PlayerActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if strReq, ok := request.(string); ok && strReq == "bind_gate" {
		pa.gatePID = from
		// 取消卸载定时器
		if pa.unloadCancel != nil {
			pa.unloadCancel()
			pa.unloadCancel = nil
			log.Printf("[PlayerActor] Player %s 重新绑定新网关 %s, 已取消下线卸载定时器", pa.playerID, from.String())
		}
		return "ok", nil
	}

	if strReq, ok := request.(string); ok && strReq == "release" {
		// 释放握手：同步全量落盘后回包退出。
		// ergo 保证 (result, TerminateReasonNormal) 先回包再终止，
		// 因此调用方收到 "released" 时数据已安全在 DB 中。
		pa.syncFlushAll()
		pa.released = true
		log.Printf("[PlayerActor] Player %s 收到释放握手, 已同步落盘, 退位销毁", pa.playerID)
		return "released", gen.TerminateReasonNormal
	}

	return "error: unhandled_call", nil
}

// flushDirty 将 Model 中的脏字段投递到全局 SaveQueue
func (pa *PlayerActor) flushDirty() {
	dirtyFields := pa.model.FlushDirty()
	if len(dirtyFields) == 0 {
		return
	}

	log.Printf("[PlayerActor] Player %s 数据变脏，投递异步落盘 (脏字段数: %d)", pa.playerID, len(dirtyFields))
	storage.SaveQueue <- storage.SaveCommand{
		PlayerID:    pa.playerID,
		Epoch:       pa.epoch,
		DirtyFields: dirtyFields,
	}
}

// syncFlushAll 同步全量落盘，阻塞直到写入完成（释放握手/Terminate 共用）
func (pa *PlayerActor) syncFlushAll() {
	dirtyFields := pa.model.ForceFlushAll()
	if len(dirtyFields) == 0 {
		return
	}

	done := make(chan error, 1)
	storage.SaveQueue <- storage.SaveCommand{
		PlayerID:    pa.playerID,
		Epoch:       pa.epoch,
		DirtyFields: dirtyFields,
		DoneCh:      done,
	}

	// 阻塞等待写入完成
	if err := <-done; err != nil {
		log.Printf("[PlayerActor] 玩家 %s 同步落盘失败: %v", pa.playerID, err)
	}
}

// handleBuildUpgrade 处理升级建筑逻辑
func (pa *PlayerActor) handleBuildUpgrade(payload []byte) {
	req := &game.C2S_BuildUpgrade{}
	if err := proto.Unmarshal(payload, req); err != nil {
		log.Printf("[PlayerActor] 解析 C2S_BuildUpgrade 失败: %v", err)
		return
	}

	// 1. 执行数值结算
	if pa.model.Profile.GetGold() < 100 {
		// 金币不足，返回失败
		pa.sendUpgradeResponse(1, req.BuildId, pa.model.Profile.GetBuildLevel())
		return
	}

	pa.model.Profile.AddGold(-100)                                       // 自动标脏 gold 位
	pa.model.Profile.SetBuildLevel(pa.model.Profile.GetBuildLevel() + 1) // 自动标脏 build_level 位
	pa.model.Profile.AddPower(250)                                       // 自动标脏 power 位

	// 更新内存中建筑玩法数据
	pa.model.Builds.SetBuildsKey(req.BuildId, pa.model.Profile.GetBuildLevel()) // 自动标脏 builds 位

	pa.dirtyGold = true  // 标记金币变动，需要差量同步客户端
	pa.dirtyPower = true // 标记战力变动，需要差量同步客户端

	log.Printf("[PlayerActor] Player %s 建筑 %s 升级成功! 当前等级: %d, 剩余金币: %d",
		pa.playerID, req.BuildId, pa.model.Profile.GetBuildLevel(), pa.model.Profile.GetGold())

	// 2. 发送业务响应
	pa.sendUpgradeResponse(0, req.BuildId, pa.model.Profile.GetBuildLevel())

	// 3. 触发内存差量增量同步
	pa.sendSyncData()

	// 4. 上报大地图服 (演示 home → world 通路, 失败不影响主流程)
	report := fmt.Sprintf("%s:lv%d", req.BuildId, pa.model.Profile.GetBuildLevel())
	pa.notifyWorld(actor.ProtoHomeBuildReport, []byte(report))
}

// notifyWorld home → world 主动上报: 按配置的节点名寻址 world_actor 投递内部协议 envelope
func (pa *PlayerActor) notifyWorld(protoID int32, payload []byte) {
	if pa.worldNode == "" {
		return // 未配置 world 节点, 静默跳过
	}

	worldActorPID := gen.ProcessID{Name: gen.Atom("world_actor"), Node: gen.Atom(pa.worldNode)}
	if err := actor.SendEnvelope(pa.Process, worldActorPID, 0, pa.playerID, protoID, payload); err != nil {
		log.Printf("[PlayerActor] Player %s 上报 world 失败 (协议 %d): %v", pa.playerID, protoID, err)
	}
}

// handleInternalMessage 处理内部协议段 (90000+) 消息: 来自 world 等后端节点
func (pa *PlayerActor) handleInternalMessage(envelope *rpc.RPCEnvelope) {
	switch envelope.ProtoId {
	case actor.ProtoWorldBuildAck:
		// world → home 建筑上报回执 (演示通路闭环, 仅打日志)
		log.Printf("[PlayerActor] Player %s 收到 world 建筑回执: %s", pa.playerID, string(envelope.Payload))
	default:
		log.Printf("[PlayerActor] Player %s 收到未识别的内部协议: %d", pa.playerID, envelope.ProtoId)
	}
}

func (pa *PlayerActor) sendUpgradeResponse(code int32, buildID string, level int32) {
	res := &game.S2C_BuildUpgradeRes{
		Code:    code,
		BuildId: buildID,
		Level:   level,
	}
	resBytes, _ := proto.Marshal(res)
	resPacket, _ := network.PackWS(int32(game.MsgID_MSG_S2C_BUILD_UPGRADE_RES), 0, resBytes)
	pa.Send(pa.gatePID, resPacket)
}

func (pa *PlayerActor) sendSyncData() {
	if !pa.dirtyGold && !pa.dirtyPower {
		return
	}

	res := &game.S2C_SyncData{}
	var modules []string

	if pa.dirtyGold {
		res.Gold = pa.model.Profile.GetGold()
		modules = append(modules, "gold")
		pa.dirtyGold = false
	}
	if pa.dirtyPower {
		res.Power = pa.model.Profile.GetPower()
		modules = append(modules, "power")
		pa.dirtyPower = false
	}
	res.DirtyModules = modules

	resBytes, _ := proto.Marshal(res)
	resPacket, _ := network.PackWS(int32(game.MsgID_MSG_S2C_SYNC_DATA), 0, resBytes)
	pa.Send(pa.gatePID, resPacket)
}

// Terminate 在销毁时执行同步写回，阻塞等待确保数据落盘
func (pa *PlayerActor) Terminate(reason error) {
	// 释放握手已在 HandleCall 中同步落盘，避免重复写入
	if pa.released {
		return
	}

	pa.syncFlushAll()
	log.Printf("[PlayerActor] 玩家 %s 最终数据同步保存落盘完成! Gold: %d, Power: %d, BuildLevel: %d, 原因: %v",
		pa.playerID, pa.model.Profile.GetGold(), pa.model.Profile.GetPower(), pa.model.Profile.GetBuildLevel(), reason)
}
