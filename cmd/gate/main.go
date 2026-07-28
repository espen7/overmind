package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"github.com/gorilla/websocket"

	"overmind/internal/app/gate"
	"overmind/internal/pkg/config"
	"overmind/internal/pkg/network"
	"overmind/internal/pkg/storage"
)

var (
	connIDCounter int64
	configPath    = flag.String("config", "configs/gate.yaml", "Path to gate configuration file")
)

func main() {
	flag.Parse()

	// 1. 载入配置文件
	cfg, err := config.LoadGateConfig(*configPath)
	if err != nil {
		log.Fatalf("载入网关配置文件失败 [%s]: %v", *configPath, err)
	}

	// 2. 配置节点选项 (ergo V3 核心配置在 gen.NodeOptions 中)
	opts := gen.NodeOptions{}
	opts.Network.Cookie = cfg.Node.Cookie

	// 配置该网关节点自身的 TCP 监听端口，用于集群内部通信
	opts.Network.Acceptors = []gen.AcceptorOptions{
		{
			Host: cfg.Node.ListenHost,
			Port: cfg.Node.ListenPort,
		},
	}

	// 3. 启动 Gate 节点
	gateNode, err := ergo.StartNode(gen.Atom(cfg.Node.Name), opts)
	if err != nil {
		log.Fatalf("启动 Gate 节点失败: %v", err)
	}
	log.Printf("Gate 节点启动成功: %s", gateNode.Name())

	// 4. 节点间寻址无需静态路由: ergo 内嵌 registrar (host:4499) 按节点名自动解析监听端口

	// 5. 构建 Home 一致性哈希环: 运行期真相源为 Mongo 环文档 (yaml 播种/兜底), 3s 轮询热切
	mongoReady := false
	if cfg.Database.URI != "" {
		if dbErr := storage.InitDB(cfg.Database.URI, cfg.Database.DBName); dbErr != nil {
			log.Printf("警告: 连接 Mongo 失败, 退化为 yaml 静态环 (无法感知在线扩缩容): %v", dbErr)
		} else {
			mongoReady = true
		}
	} else {
		log.Printf("警告: 未配置 database, 退化为 yaml 静态环 (无法感知在线扩缩容)")
	}

	ringMgr := storage.BuildRingManager(cfg.HomeRing.Nodes, cfg.HomeRing.Version, cfg.HomeRing.PrevNodes, "gate")
	if ringMgr == nil {
		log.Fatalf("配置错误: Mongo 环文档与 home_ring.nodes 均为空, 无法构建哈希环")
	}
	if mongoReady {
		storage.StartRingPoller(ringMgr, 3*time.Second, "gate")
	}

	// 手动换环应急入口 (常规改环走 ringctl 写 Mongo 文档 + 轮询热切)
	_, adminErr := gateNode.SpawnRegister(gen.Atom("gate_ring_admin"), func() gen.ProcessBehavior {
		return &gate.RingAdminActor{}
	}, gen.ProcessOptions{}, ringMgr)
	if adminErr != nil {
		log.Fatalf("派生 RingAdminActor 失败: %v", adminErr)
	}

	// 6. 启动 WebSocket 网关服务
	wsServer := network.NewWSServer(cfg.WebSocket.ListenAddr, func(conn *websocket.Conn) {
		id := atomic.AddInt64(&connIDCounter, 1)
		actorName := fmt.Sprintf("conn_%d", id)

		// 使用 SpawnRegister 启动长连接对应的 ChannelActor
		pid, spawnErr := gateNode.SpawnRegister(gen.Atom(actorName), func() gen.ProcessBehavior {
			return &gate.ChannelActor{}
		}, gen.ProcessOptions{}, id, conn, ringMgr, cfg.WorldNode)

		if spawnErr != nil {
			log.Printf("为连接 ID %d 派生 ChannelActor 失败: %v", id, spawnErr)
			conn.Close()
			return
		}
		log.Printf("为连接 ID %d 成功派生 ChannelActor: %s", id, pid.String())
	})

	// 7. 阻塞运行网关服务
	err = wsServer.Start()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("WebSocket 网关监听服务异常终止: %v", err)
	}
}
