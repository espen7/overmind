package gate

import (
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
)

// ChannelActor 维护每个客户端连接的生命周期和状态路由
type ChannelActor struct {
	act.Actor
	connID     int64
	playerID   string
	wsConn     *websocket.Conn
	homePID    gen.PID        // 逻辑服上的 Player Actor PID
	worldPID   gen.PID        // 地图服上的 Cell Actor PID (暂存)
	subChunks  map[int32]bool // 玩家当前订阅的地图区块 (Chunk) ID
	lastHeart  time.Time
	authStatus bool
}

// Init 初始化 Channel Actor (实现 act.ActorBehavior 接口)
func (ca *ChannelActor) Init(args ...any) error {
	if len(args) < 2 {
		return fmt.Errorf("bad_arguments")
	}

	connID, ok1 := args[0].(int64)
	wsConn, ok2 := args[1].(*websocket.Conn)
	if !ok1 || !ok2 {
		return fmt.Errorf("bad_arguments")
	}

	ca.connID = connID
	ca.wsConn = wsConn
	ca.subChunks = make(map[int32]bool)
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

// HandleEvent 处理订阅的地图 Chunk 广播事件
func (ca *ChannelActor) HandleEvent(message gen.MessageEvent) error {
	log.Printf("[ChannelActor] ConnID %d 收到地图区块广播事件, ChunkID: %v, 内容: %v",
		ca.connID, message.Event.Name, message.Message)

	// 将地图广播消息拼装为二进制并推送给客户端
	resPacket, _ := network.PackWS(int32(game.MsgID_MSG_S2C_MAP_BROADCAST), 0, []byte(fmt.Sprintf("map_event:%v", message.Message)))
	_ = ca.wsConn.WriteMessage(websocket.BinaryMessage, resPacket)
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

		// 一致性哈希路由目标逻辑节点。在分布式下，通过 ProcessID 将请求定向到对应节点的 home_coordinator
		homeCoordinatorPID := gen.ProcessID{
			Name: gen.Atom("home_coordinator"),
			Node: gen.Atom("home1@127.0.0.1"),
		}

		// 通过同步 Call 分配/创建 PlayerActor 并返回 PID。直接传入 string 类型的 ca.playerID
		log.Printf("[ChannelActor] 发起跨节点同步调用分配 PlayerActor...")
		resTerm, err := ca.Call(homeCoordinatorPID, ca.playerID)
		if err != nil {
			log.Printf("[ChannelActor] 分配 PlayerActor 失败: %v", err)
			ca.sendLoginRes(1, "server_error")
			return
		}

		if playerPID, ok := resTerm.(gen.PID); ok {
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

			log.Printf("[ChannelActor] Player %s 登录成功，绑定 HomePID: %s", ca.playerID, ca.homePID.String())
			ca.sendLoginRes(0, "success")
		} else {
			ca.sendLoginRes(2, "allocate_failed")
		}
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

	// 3. 路由分发大地图视野订阅 (AOI 模拟) (ProtoID: 10010 模拟订阅 Chunk 101)
	if packet.ProtoID == int32(game.MsgID_MSG_C2S_SUBSCRIBE_MAP) {
		// 使用 ergo 原生的 LinkEvent 订阅大地图 Chunk 101 事件
		// V3 的 Event 仅需通过事件名及目标节点名(World 节点)来定位订阅，无需进行 PID 解析
		_, err := ca.LinkEvent(gen.Event{Name: gen.Atom("chunk_101"), Node: gen.Atom("world@127.0.0.1")})
		if err != nil {
			log.Printf("[ChannelActor] 订阅 chunk_101 视野失败: %v", err)
		} else {
			ca.subChunks[101] = true
			log.Printf("[ChannelActor] 成功订阅大地图 chunk_101 视野总线")
		}
		return
	}

	// 4. 路由分发到逻辑服 (玩家个人业务协议 10000 - 19999)
	if packet.ProtoID >= 10000 && packet.ProtoID < 20000 {
		if !ca.authStatus || ca.homePID.Node == "" {
			log.Printf("[ChannelActor] 未登录授权，拒绝转发路由协议: %d", packet.ProtoID)
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

	log.Printf("[ChannelActor] 未识别的协议 ID: %d", packet.ProtoID)
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
