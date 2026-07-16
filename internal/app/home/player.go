package home

import (
	"fmt"
	"log"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"google.golang.org/protobuf/proto"

	"overmind/api/game"
	"overmind/api/rpc"
	"overmind/internal/pkg/network"
	"overmind/internal/pkg/storage"
)

// ==================== HomeCoordinatorActor ====================

// HomeCoordinatorActor 管理本地节点所有的 PlayerActor 实例
type HomeCoordinatorActor struct {
	act.Actor
}

// Init 初始化协调器
func (hc *HomeCoordinatorActor) Init(args ...any) error {
	log.Printf("[HomeCoordinatorActor] 协调器启动成功")
	return nil
}

// HandleCall 响应同步请求 (用于分配/创建 PlayerActor)
func (hc *HomeCoordinatorActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// 使用 Go 原生基本类型 string 作为请求入参，避免序列化配置开销
	playerID, ok := request.(string)
	if !ok {
		return nil, fmt.Errorf("invalid_request_type")
	}

	actorName := fmt.Sprintf("player_actor_%s", playerID)

	// 1. 检查是否已经创建过该玩家的 Actor
	pid, err := hc.Node().ProcessPID(gen.Atom(actorName))
	if err == nil {
		log.Printf("[HomeCoordinatorActor] Player %s 已经在线/在内存缓冲中, 直接返回 PID: %s", playerID, pid.String())
		return pid, nil
	}

	// 2. 惰性加载：玩家不存在，直接 Spawn 创建一个新的 PlayerActor
	newPID, err := hc.SpawnRegister(gen.Atom(actorName), func() gen.ProcessBehavior {
		return &PlayerActor{}
	}, gen.ProcessOptions{}, playerID)

	if err != nil {
		log.Printf("[HomeCoordinatorActor] 启动 PlayerActor 失败: %v", err)
		return nil, err
	}

	log.Printf("[HomeCoordinatorActor] 成功为 Player %s 创建专属 PlayerActor: %s", playerID, newPID.String())
	return newPID, nil
}

// ==================== PlayerActor ====================

// PlayerActor 玩家个人逻辑状态机，生命周期与 Socket 网关 Actor Link 绑定
type PlayerActor struct {
	act.Actor
	playerID string
	gatePID  gen.PID // 绑定的网关 Channel Actor PID

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
	if len(args) < 1 {
		return fmt.Errorf("bad_arguments")
	}

	playerID, ok := args[0].(string)
	if !ok {
		return fmt.Errorf("bad_arguments")
	}

	pa.playerID = playerID

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

	log.Printf("[PlayerActor] 玩家 %s 状态机载入成功 (Gold: %d, Power: %d, BuildLevel: %d)",
		pa.playerID, pa.model.Profile.GetGold(), pa.model.Profile.GetPower(), pa.model.Profile.GetBuildLevel())

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
		// 解包网关转发的 RPCEnvelope 数据包
		envelope := &rpc.RPCEnvelope{}
		if err := proto.Unmarshal(msg, envelope); err != nil {
			log.Printf("[PlayerActor] 解包 RPCEnvelope 错误: %v", err)
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

// HandleCall 处理同步调用 (实现重连网关的重新绑定)
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
		DirtyFields: dirtyFields,
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
	dirtyFields := pa.model.ForceFlushAll()
	if len(dirtyFields) == 0 {
		return
	}

	done := make(chan error, 1)
	storage.SaveQueue <- storage.SaveCommand{
		PlayerID:    pa.playerID,
		DirtyFields: dirtyFields,
		DoneCh:      done,
	}

	// 阻塞等待写入完成
	err := <-done
	if err != nil {
		log.Printf("[PlayerActor] 玩家 %s Terminate 落盘失败: %v", pa.playerID, err)
	} else {
		log.Printf("[PlayerActor] 玩家 %s 最终数据同步保存落盘成功! Gold: %d, Power: %d, BuildLevel: %d, 原因: %v",
			pa.playerID, pa.model.Profile.GetGold(), pa.model.Profile.GetPower(), pa.model.Profile.GetBuildLevel(), reason)
	}
}
