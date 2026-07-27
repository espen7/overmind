package gate

import (
	"errors"
	"fmt"
	"log"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"overmind/api/game"
	"overmind/internal/pkg/actor"
	"overmind/internal/pkg/network"
	"overmind/internal/pkg/routing"
)

// 分配失败的可识别错误: 登录路径据此给客户端友好提示
var (
	errEmptyRing     = errors.New("empty_home_ring")
	errRingSwitching = errors.New("ring_switching") // 协调器 guard 拒绝: 环切换收敛中 (轮询周期内自愈)
)

// ChannelActor 维护每个客户端连接的生命周期和状态路由
type ChannelActor struct {
	act.Actor
	connID     int64
	playerID   string
	wsConn     *websocket.Conn
	ringMgr    *routing.Manager // Home 节点一致性哈希环（全局共享，支持在线换环）
	worldNode  string           // World 节点名 (来自配置, 寻址 world_actor)
	homePID    gen.PID          // 逻辑服上的 Player Actor PID
	lastHeart  time.Time
	authStatus bool
}

// Init 初始化 Channel Actor (实现 act.ActorBehavior 接口)
func (ca *ChannelActor) Init(args ...any) error {
	if len(args) < 4 {
		return fmt.Errorf("bad_arguments")
	}

	connID, ok1 := args[0].(int64)
	wsConn, ok2 := args[1].(*websocket.Conn)
	ringMgr, ok3 := args[2].(*routing.Manager)
	worldNode, ok4 := args[3].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return fmt.Errorf("bad_arguments")
	}

	ca.connID = connID
	ca.wsConn = wsConn
	ca.ringMgr = ringMgr
	ca.worldNode = worldNode
	ca.lastHeart = time.Now()

	// 启动后台连接读取循环 (WebSocket 阻塞读取)
	go ca.readLoop(ca.Process, wsConn)

	log.Printf("[ChannelActor] 连接 ID %d 初始化成功, 开启监听协程", connID)
	return nil
}

// HandleCall 处理同步调用 (暂无同步业务)
func (ca *ChannelActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	log.Printf("[ChannelActor] 收到来自 %s 的未处理同步请求: %v", from.String(), request)
	return "error: unhandled_call", nil
}

// HandleMessage 处理异步普通消息 (实现事件分发)
func (ca *ChannelActor) HandleMessage(from gen.PID, message any) error {
	log.Printf("[ChannelActor] HandleMessage 收到消息来自 %s, 类型: %T", from.String(), message)

	switch msg := message.(type) {
	case *network.Packet:
		log.Printf("[ChannelActor] 收到网络数据包，ProtoID: %d", msg.ProtoID)
		ca.handleClientPacket(msg)
	case []byte:
		log.Printf("[ChannelActor] 收到后端同步字节流数据，大小: %d", len(msg))
		// 收到后端逻辑服/地图服跨进程发过来的增量数据同步推送包 (S2C_SyncData 等)
		// 直接通过 WebSocket 发送给客户端
		err := ca.wsConn.WriteMessage(websocket.BinaryMessage, msg)
		if err != nil {
			log.Printf("[ChannelActor] ConnID %d 写入客户端失败: %v", ca.connID, err)
			return err // 返回错误会导致该 Actor 退出
		}
	default:
		log.Printf("[ChannelActor] 收到未知类型消息: %v", msg)
	}

	return nil
}

// Terminate 销毁处理，进行连接回收与退出自愈
func (ca *ChannelActor) Terminate(reason error) {
	log.Printf("[ChannelActor] ConnID %d 正在终止，原因: %v", ca.connID, reason)
	if ca.wsConn != nil {
		ca.wsConn.Close()
	}
}

// readLoop WebSocket 阻塞读取的 Goroutine
func (ca *ChannelActor) readLoop(process gen.Process, conn *websocket.Conn) {
	selfPID := process.PID()
	node := process.Node()

	defer func() {
		// 物理 Socket 断开，使用 Node.SendExit 优雅发送退出信号，不依赖 process 状态
		_ = node.SendExit(selfPID, fmt.Errorf("socket_closed"))
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[ChannelActor] WS 读取异常，准备断开连接: %v", err)
			break
		}

		// 二进制解包
		packet, err := network.UnpackWS(data)
		if err != nil {
			log.Printf("[ChannelActor] 二进制包解析错误: %v", err)
			continue
		}

		log.Printf("[ChannelActor-WS] 读取到套接字包, ProtoID: %d", packet.ProtoID)

		// 使用 Node.Send 投递包到信箱，避免 process 状态限制
		err = node.Send(selfPID, packet)
		if err != nil {
			log.Printf("[ChannelActor-WS] 投递包到信箱失败: %v", err)
		}
	}
}

// handleClientPacket 处理具体的客户端请求
func (ca *ChannelActor) handleClientPacket(packet *network.Packet) {
	ca.lastHeart = time.Now()
	log.Printf("[ChannelActor] 开始处理客户端请求, ProtoID: %d", packet.ProtoID)

	// 1. 处理登录请求 (ProtoID: 1001)
	if packet.ProtoID == int32(game.MsgID_MSG_C2S_LOGIN) {
		req := &game.C2S_Login{}
		if err := proto.Unmarshal(packet.Payload, req); err != nil {
			log.Printf("[ChannelActor] 解析 C2S_Login 协议失败: %v", err)
			return
		}

		ca.playerID = req.PlayerId
		ca.authStatus = true

		if err := ca.allocateHome(); err != nil {
			// 按失败原因给客户端可区分的提示: 环切换收敛中属短暂状态, 客户端稍后重试即可
			msg := "server_error"
			switch {
			case errors.Is(err, errRingSwitching):
				msg = "server_scaling_retry_later"
			case errors.Is(err, errEmptyRing):
				msg = "server_not_ready"
			}
			ca.sendLoginRes(1, msg)
			return
		}

		log.Printf("[ChannelActor] Player %s 登录成功，绑定 HomePID: %s", ca.playerID, ca.homePID.String())
		ca.sendLoginRes(0, "success")
		return
	}

	// 2. 处理心跳包 (ProtoID: 1003)
	if packet.ProtoID == int32(game.MsgID_MSG_C2S_HEARTBEAT) {
		req := &game.C2S_Heartbeat{}
		if err := proto.Unmarshal(packet.Payload, req); err == nil {
			res := &game.S2C_HeartbeatRes{ServerTime: time.Now().UnixNano()}
			resData, _ := network.PackWS(int32(game.MsgID_MSG_S2C_HEARTBEAT_RES), packet.SeqID, ca.mustMarshal(res))
			_ = ca.wsConn.WriteMessage(websocket.BinaryMessage, resData)
			log.Printf("[ChannelActor] 已发送心跳回包给客户端")
		}
		return
	}

	// 3. 路由分发到逻辑服 (玩家个人业务协议 10000 - 19999)
	if packet.ProtoID >= 10000 && packet.ProtoID < 20000 {
		if !ca.authStatus {
			log.Printf("[ChannelActor] 未登录授权，拒绝转发路由协议: %d", packet.ProtoID)
			return
		}

		// 每包校验归属：环切换后玩家归属变化时，自动向新节点重新分配并重新绑定
		if err := ca.ensureHomeBinding(); err != nil {
			log.Printf("[ChannelActor] 重新绑定 Home 节点失败，丢弃协议 %d: %v", packet.ProtoID, err)
			return
		}

		// 使用 RPCEnvelope 跨进程将 Payload 投递给 PlayerActor
		log.Printf("[ChannelActor] 转发业务数据包 %d 到 PlayerActor...", packet.ProtoID)
		err := actor.SendEnvelope(ca.Process, ca.homePID, ca.connID, ca.playerID, packet.ProtoID, packet.Payload)
		if err != nil {
			log.Printf("[ChannelActor] 路由 RPC 到 Home 逻辑节点失败: %v", err)
		}
		return
	}

	// 4. 路由分发到大地图服 (world 协议段 20000 - 29999)
	if packet.ProtoID >= 20000 && packet.ProtoID < 30000 {
		if !ca.authStatus {
			log.Printf("[ChannelActor] 未登录授权，拒绝转发大地图协议: %d", packet.ProtoID)
			return
		}

		// world 单节点固定寻址 (注册名 + 配置节点名); world 回包直接投递本 Actor 直透客户端
		worldActorPID := gen.ProcessID{Name: gen.Atom("world_actor"), Node: gen.Atom(ca.worldNode)}
		log.Printf("[ChannelActor] 转发大地图数据包 %d 到 WorldActor...", packet.ProtoID)
		err := actor.SendEnvelope(ca.Process, worldActorPID, ca.connID, ca.playerID, packet.ProtoID, packet.Payload)
		if err != nil {
			log.Printf("[ChannelActor] 路由 RPC 到 World 地图节点失败: %v", err)
		}
		return
	}

	log.Printf("[ChannelActor] 未识别的协议 ID: %d", packet.ProtoID)
}

// allocateHome 按当前哈希环定位归属节点，分配 PlayerActor 并完成 Link + bind_gate 绑定
func (ca *ChannelActor) allocateHome() error {
	// 一致性哈希路由目标逻辑节点：任何网关对同一 PlayerID 算出的节点都相同
	targetNode := ca.ringMgr.PickCurrent(ca.playerID)
	if targetNode == "" {
		return errEmptyRing
	}

	homeCoordinatorPID := gen.ProcessID{
		Name: gen.Atom("home_coordinator"),
		Node: gen.Atom(targetNode),
	}

	// 同步 Call 分配/创建 PlayerActor。迁移场景下协调器内部会多一次释放握手,
	// 超时链对齐: 本层 15s > 新协调器→旧协调器 10s > 旧协调器→PlayerActor 8s
	log.Printf("[ChannelActor] 发起跨节点同步调用分配 PlayerActor (目标节点: %s)...", targetNode)
	resTerm, err := ca.CallWithTimeout(homeCoordinatorPID, ca.playerID, 15)
	if err != nil {
		log.Printf("[ChannelActor] 分配 PlayerActor 失败: %v", err)
		return err
	}

	playerPID, ok := resTerm.(gen.PID)
	if !ok {
		// 协调器 guard 拒绝: 本网关持旧环找错了节点, 等轮询热切 (≤3s) 后重试即可
		if res, isStr := resTerm.(string); isStr && res == "error: wrong_owner" {
			log.Printf("[ChannelActor] Player %s 分配被拒 (环切换收敛中): 目标节点 %s 认为归属已变更", ca.playerID, targetNode)
			return errRingSwitching
		}
		return fmt.Errorf("allocate_failed: %v", resTerm)
	}

	ca.homePID = playerPID
	// 原生生死 Link 绑定：连接断开时，后端 PlayerActor 自动清理释放
	ca.Link(ca.homePID)

	// 发送唤醒同步调用，通知 PlayerActor 绑定新网关并取消注销定时器
	_, errBind := ca.Call(playerPID, "bind_gate")
	if errBind != nil {
		log.Printf("[ChannelActor] 发送 bind_gate 唤醒失败: %v", errBind)
	} else {
		log.Printf("[ChannelActor] 成功通过 bind_gate 唤醒激活后端 PlayerActor")
	}
	return nil
}

// ensureHomeBinding 校验当前绑定的 Home 节点是否仍为哈希环归属节点；
// 环切换导致归属变化时，先解除旧 Link 再向新节点重新分配（旧 Actor 由释放握手回收）
func (ca *ChannelActor) ensureHomeBinding() error {
	targetNode := ca.ringMgr.PickCurrent(ca.playerID)
	if ca.homePID.Node != "" && string(ca.homePID.Node) == targetNode {
		return nil
	}

	if ca.homePID.Node != "" {
		log.Printf("[ChannelActor] 检测到哈希环切换: Player %s 归属 %s -> %s, 触发在线迁移",
			ca.playerID, ca.homePID.Node, targetNode)
		// 先解除生死绑定，避免旧 Actor 被释放握手回收时连带杀掉本连接
		_ = ca.Unlink(ca.homePID)
		ca.homePID = gen.PID{}
	}

	return ca.allocateHome()
}

func (ca *ChannelActor) sendLoginRes(code int32, msg string) {
	res := &game.S2C_LoginRes{Code: code, Msg: msg}
	resBytes, _ := proto.Marshal(res)
	resData, _ := network.PackWS(int32(game.MsgID_MSG_S2C_LOGIN_RES), 0, resBytes)
	_ = ca.wsConn.WriteMessage(websocket.BinaryMessage, resData)
}

func (ca *ChannelActor) mustMarshal(msg proto.Message) []byte {
	data, _ := proto.Marshal(msg)
	return data
}
