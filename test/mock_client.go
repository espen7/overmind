package main

import (
	"log"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"overmind/api/game"
	"overmind/internal/pkg/network"
)

// readLoop 后台消息读取循环: 按协议 ID 解析并打印各类下行包
func readLoop(sessionName string, conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[%s Reader] 连接已断开: %v", sessionName, err)
			return
		}

		packet, err := network.UnpackWS(data)
		if err != nil {
			log.Printf("[%s Reader] 解析回包错误: %v", sessionName, err)
			continue
		}

		switch packet.ProtoID {
		case int32(game.MsgID_MSG_S2C_LOGIN_RES):
			res := &game.S2C_LoginRes{}
			_ = proto.Unmarshal(packet.Payload, res)
			log.Printf("[%s Reader] 收到登录响应 -> Code: %d, Msg: %s", sessionName, res.Code, res.Msg)
		case int32(game.MsgID_MSG_S2C_HEARTBEAT_RES):
			res := &game.S2C_HeartbeatRes{}
			_ = proto.Unmarshal(packet.Payload, res)
			log.Printf("[%s Reader] 收到心跳响应 -> ServerTime: %d", sessionName, res.ServerTime)
		case int32(game.MsgID_MSG_S2C_BUILD_UPGRADE_RES):
			res := &game.S2C_BuildUpgradeRes{}
			_ = proto.Unmarshal(packet.Payload, res)
			log.Printf("[%s Reader] 收到建筑升级响应 -> Code: %d, BuildId: %s, Level: %d", sessionName, res.Code, res.BuildId, res.Level)
		case int32(game.MsgID_MSG_S2C_SYNC_DATA):
			res := &game.S2C_SyncData{}
			_ = proto.Unmarshal(packet.Payload, res)
			log.Printf("[%s Reader] 收到增量脏数据同步 (S2C_SyncData) -> Gold: %d, Power: %d, DirtyModules: %v", sessionName, res.Gold, res.Power, res.DirtyModules)
		case int32(game.MsgID_MSG_S2C_WORLD_PONG):
			res := &game.S2C_WorldPong{}
			_ = proto.Unmarshal(packet.Payload, res)
			log.Printf("[%s Reader] 收到大地图 Pong -> 坐标: (%d, %d), Msg: %s, ServerTime: %d", sessionName, res.X, res.Y, res.Msg, res.ServerTime)
		default:
			log.Printf("[%s Reader] 收到其他数据包 -> ProtoID: %d, 载荷内容: %s", sessionName, packet.ProtoID, string(packet.Payload))
		}
	}
}

func runClientSession(sessionName string, autoDisconnectSec int, shouldUpgrade bool) {
	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8080", Path: "/ws"}
	log.Printf("[%s] 正在连接网关 %s...", sessionName, u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("[%s] 连接失败: %v", sessionName, err)
	}
	defer conn.Close()
	log.Printf("[%s] 连接建立成功!", sessionName)

	// 启动后台消息读取协程
	go readLoop(sessionName, conn)

	// 1. 发送 C2S_Login (ProtoID: 1001)
	log.Printf("[%s] 发送 C2S_Login...", sessionName)
	loginReq := &game.C2S_Login{PlayerId: "test_player_999"}
	loginBytes, _ := proto.Marshal(loginReq)
	loginFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_LOGIN), 1, loginBytes)
	_ = conn.WriteMessage(websocket.BinaryMessage, loginFrame)

	time.Sleep(1 * time.Second)

	if shouldUpgrade {
		// 2. 发送 C2S_BuildUpgrade (ProtoID: 10005)
		log.Printf("[%s] 发送 C2S_BuildUpgrade...", sessionName)
		upgradeReq := &game.C2S_BuildUpgrade{BuildId: "barracks_1"}
		upgradeBytes, _ := proto.Marshal(upgradeReq)
		upgradeFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_BUILD_UPGRADE), 2, upgradeBytes)
		_ = conn.WriteMessage(websocket.BinaryMessage, upgradeFrame)
		time.Sleep(1 * time.Second)
	}

	// 3. 发送大地图探活 (ProtoID: 20001, 验证 gate ↔ world 转发闭环)
	log.Printf("[%s] 发送大地图 C2S_WorldPing...", sessionName)
	pingReq := &game.C2S_WorldPing{X: 100, Y: 200}
	pingBytes, _ := proto.Marshal(pingReq)
	pingFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_WORLD_PING), 3, pingBytes)
	_ = conn.WriteMessage(websocket.BinaryMessage, pingFrame)

	// 4. 发送心跳 C2S_Heartbeat
	log.Printf("[%s] 发送心跳 C2S_Heartbeat...", sessionName)
	hbReq := &game.C2S_Heartbeat{}
	hbBytes, _ := proto.Marshal(hbReq)
	hbFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_HEARTBEAT), 4, hbBytes)
	_ = conn.WriteMessage(websocket.BinaryMessage, hbFrame)

	log.Printf("[%s] 等待收包中...", sessionName)
	time.Sleep(time.Duration(autoDisconnectSec) * time.Second)

	log.Printf("[%s] 会话结束，主动断开连接...", sessionName)
}

func main() {
	// scale 模式: 配合 scripts/run_scale_test.ps1 验证双 home 在线扩容剧本
	if len(os.Args) > 1 && os.Args[1] == "scale" {
		runScaleScenario()
		return
	}

	// shrink 模式: 配合 scripts/run_shrink_test.ps1 验证扩容+缩容(节点退役)全剧本
	if len(os.Args) > 1 && os.Args[1] == "shrink" {
		runShrinkScenario()
		return
	}

	log.Printf("================ [测试阶段 1: 首次登录与升级] ================")
	runClientSession("Client-Session-1", 2, true)

	log.Printf("==== [等待 2 秒（小于 5 秒卸载时长），模拟临时网络抖动后发起断线重连] ====")
	time.Sleep(2 * time.Second)

	log.Printf("================ [测试阶段 2: 重新登录并重连绑定] ================")
	// 重新登录，此操作应触发 bind_gate 并撤销卸载定时器
	runClientSession("Client-Session-2", 3, false)

	log.Printf("================ [测试阶段 3: 最终断开连接，等待 6 秒验证超时卸载与 MongoDB 落盘] ================")
	time.Sleep(6 * time.Second)
	log.Printf("测试流程执行完成，退出。")
}

// runScaleScenario 扩容剧本客户端: 全程保持同一条连接,
// 在第一次升级后留出窗口给外部执行 ringctl add, 第二次升级应触发在线迁移到新节点
func runScaleScenario() {
	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8080", Path: "/ws"}
	log.Printf("[Scale] 正在连接网关 %s...", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("[Scale] 连接失败: %v", err)
	}
	defer conn.Close()

	go readLoop("Scale", conn)

	// 1. 登录 (此时环为 [home1], test_player_999 归属 home1)
	log.Printf("[Scale] 发送 C2S_Login...")
	loginReq := &game.C2S_Login{PlayerId: "test_player_999"}
	loginBytes, _ := proto.Marshal(loginReq)
	loginFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_LOGIN), 1, loginBytes)
	_ = conn.WriteMessage(websocket.BinaryMessage, loginFrame)
	time.Sleep(1 * time.Second)

	// 2. 扩容前第一次升级 (在 home1 上处理)
	log.Printf("[Scale] 扩容前发送第一次 C2S_BuildUpgrade...")
	sendUpgrade(conn, 2)

	// 3. 留窗口给外部执行 ringctl add home2 (各节点 ≤3s 轮询热切)
	log.Printf("[Scale] 等待 10s (此窗口内外部执行 ringctl add, 环切换后归属变为 home2)...")
	time.Sleep(10 * time.Second)

	// 4. 扩容后第二次升级: 网关每包校验应检测到归属变化, 触发释放握手迁移到 home2
	log.Printf("[Scale] 扩容后发送第二次 C2S_BuildUpgrade (应触发在线迁移)...")
	sendUpgrade(conn, 3)

	// 5. 心跳验证连接仍健康
	hbReq := &game.C2S_Heartbeat{}
	hbBytes, _ := proto.Marshal(hbReq)
	hbFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_HEARTBEAT), 4, hbBytes)
	_ = conn.WriteMessage(websocket.BinaryMessage, hbFrame)

	time.Sleep(2 * time.Second)
	log.Printf("[Scale] 扩容剧本客户端流程完成, 退出。")
}

func sendUpgrade(conn *websocket.Conn, seq int32) {
	upgradeReq := &game.C2S_BuildUpgrade{BuildId: "barracks_1"}
	upgradeBytes, _ := proto.Marshal(upgradeReq)
	upgradeFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_BUILD_UPGRADE), seq, upgradeBytes)
	_ = conn.WriteMessage(websocket.BinaryMessage, upgradeFrame)
	time.Sleep(1 * time.Second)
}

// runShrinkScenario 扩容+缩容全剧本客户端: 全程保持同一条连接,
// 三段升级中间留两个窗口给外部改环:
// 升级①@home1 -> [ringctl add home2] -> 升级②迁往 home2 -> [ringctl remove home2] -> 升级③迁回 home1
func runShrinkScenario() {
	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8080", Path: "/ws"}
	log.Printf("[Shrink] 正在连接网关 %s...", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("[Shrink] 连接失败: %v", err)
	}
	defer conn.Close()

	go readLoop("Shrink", conn)

	// 1. 登录 (此时环为 [home1], test_player_999 归属 home1)
	log.Printf("[Shrink] 发送 C2S_Login...")
	loginReq := &game.C2S_Login{PlayerId: "test_player_999"}
	loginBytes, _ := proto.Marshal(loginReq)
	loginFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_LOGIN), 1, loginBytes)
	_ = conn.WriteMessage(websocket.BinaryMessage, loginFrame)
	time.Sleep(1 * time.Second)

	// 2. 扩容前第一次升级 (在 home1 上处理, 期望 Lv2)
	log.Printf("[Shrink] 阶段① 发送第一次 C2S_BuildUpgrade (home1)...")
	sendUpgrade(conn, 2)

	// 3. 窗口 A: 外部执行 ringctl add home2 (各节点 <=3s 轮询热切)
	log.Printf("[Shrink] 等待 10s (窗口 A: 外部执行 ringctl add home2, 归属变为 home2)...")
	time.Sleep(10 * time.Second)

	// 4. 扩容后第二次升级: 触发在线迁移 home1 -> home2 (期望 Lv3)
	log.Printf("[Shrink] 阶段② 发送第二次 C2S_BuildUpgrade (应迁移至 home2)...")
	sendUpgrade(conn, 3)

	// 5. 窗口 B: 外部先 commit 扩容, 再执行 ringctl remove home2 (缩容)
	log.Printf("[Shrink] 等待 10s (窗口 B: 外部执行 ringctl remove home2, 归属迁回 home1)...")
	time.Sleep(10 * time.Second)

	// 6. 缩容后第三次升级: 反向握手, home1 从 home2 手里把玩家接回来 (期望 Lv4)
	log.Printf("[Shrink] 阶段③ 发送第三次 C2S_BuildUpgrade (应迁回 home1)...")
	sendUpgrade(conn, 4)

	// 7. 心跳验证连接仍健康
	hbReq := &game.C2S_Heartbeat{}
	hbBytes, _ := proto.Marshal(hbReq)
	hbFrame, _ := network.PackWS(int32(game.MsgID_MSG_C2S_HEARTBEAT), 5, hbBytes)
	_ = conn.WriteMessage(websocket.BinaryMessage, hbFrame)

	time.Sleep(2 * time.Second)
	log.Printf("[Shrink] 缩容剧本客户端流程完成, 退出。")
}
