package main

import (
	"flag"
	"log"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"

	"overmind/internal/app/world"
	"overmind/internal/pkg/config"
)

var (
	configPath = flag.String("config", "configs/world.yaml", "Path to world configuration file")
)

func main() {
	flag.Parse()

	// 1. 载入配置文件
	cfg, err := config.LoadWorldConfig(*configPath)
	if err != nil {
		log.Fatalf("载入世界服配置文件失败 [%s]: %v", *configPath, err)
	}

	// 2. 配置节点选项 (ergo V3)
	opts := gen.NodeOptions{}
	opts.Network.Cookie = cfg.Node.Cookie

	// 配置监听端口
	opts.Network.Acceptors = []gen.AcceptorOptions{
		{
			Host: cfg.Node.ListenHost,
			Port: cfg.Node.ListenPort,
		},
	}

	// 3. 启动 World 节点
	worldNode, err := ergo.StartNode(gen.Atom(cfg.Node.Name), opts)
	if err != nil {
		log.Fatalf("启动 World 节点失败: %v", err)
	}
	log.Printf("World 节点启动成功: %s", worldNode.Name())

	// 4. 静态连接寻路路由配置
	for _, r := range cfg.Routes {
		route := gen.NetworkRoute{
			Route: gen.Route{
				Host: r.Host,
				Port: r.Port,
			},
		}
		err = worldNode.Network().AddRoute(r.NodeName, route, 1)
		if err != nil {
			log.Printf("添加去往节点 %s 的路由失败: %v", r.NodeName, err)
		} else {
			log.Printf("成功添加去往节点 %s (%s:%d) 的静态路由", r.NodeName, r.Host, r.Port)
		}
	}

	// 5. Spawn 启动 Cell 1 对应的 CellActor ("cell_1")
	cellName := "cell_1"
	pid, spawnErr := worldNode.SpawnRegister(gen.Atom(cellName), func() gen.ProcessBehavior {
		return &world.CellActor{}
	}, gen.ProcessOptions{})

	if spawnErr != nil {
		log.Fatalf("派生 CellActor 失败: %v", spawnErr)
	}
	log.Printf("成功派生 CellActor: %s", pid.String())

	// 6. 模拟大地图每隔几秒产生一次行军位置变更，广播给订阅 chunk_101 的连接 Actor
	go func() {
		for {
			time.Sleep(3 * time.Second)
			log.Printf("[World Node] 定时向 cell_1 发送 trigger_march_sync 指令...")
			_ = worldNode.Send(pid, "trigger_march_sync")
		}
	}()

	// 7. 阻塞主线程保持运行
	select {}
}
