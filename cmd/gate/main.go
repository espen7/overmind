package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"github.com/gorilla/websocket"

	"overmind/internal/app/gate"
	"overmind/internal/pkg/config"
	"overmind/internal/pkg/network"
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

	// 4. 配置静态节点路由，以便节点之间能相互连接寻路 (无需 etcd 依赖)
	for _, r := range cfg.Routes {
		route := gen.NetworkRoute{
			Route: gen.Route{
				Host: r.Host,
				Port: r.Port,
			},
		}
		err = gateNode.Network().AddRoute(r.NodeName, route, 1)
		if err != nil {
			log.Printf("添加去往节点 %s 的路由失败: %v", r.NodeName, err)
		} else {
			log.Printf("成功添加去往节点 %s (%s:%d) 的静态路由", r.NodeName, r.Host, r.Port)
		}
	}

	// 5. 启动 WebSocket 网关服务
	wsServer := network.NewWSServer(cfg.WebSocket.ListenAddr, func(conn *websocket.Conn) {
		id := atomic.AddInt64(&connIDCounter, 1)
		actorName := fmt.Sprintf("conn_%d", id)

		// 使用 SpawnRegister 启动长连接对应的 ChannelActor
		pid, spawnErr := gateNode.SpawnRegister(gen.Atom(actorName), func() gen.ProcessBehavior {
			return &gate.ChannelActor{}
		}, gen.ProcessOptions{}, id, conn)

		if spawnErr != nil {
			log.Printf("为连接 ID %d 派生 ChannelActor 失败: %v", id, spawnErr)
			conn.Close()
			return
		}
		log.Printf("为连接 ID %d 成功派生 ChannelActor: %s", id, pid.String())
	})

	// 6. 阻塞运行网关服务
	err = wsServer.Start()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("WebSocket 网关监听服务异常终止: %v", err)
	}
}
