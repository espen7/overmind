package main

import (
	"flag"
	"log"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"

	"overmind/internal/app/world"
	"overmind/internal/pkg/config"
	"overmind/internal/pkg/storage"
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

	// 5. 构建 Home 哈希环 (world → home 主动通知的归属寻址):
	// 运行期真相源为 Mongo 环文档 (yaml 播种/兜底), 3s 轮询热切
	mongoReady := false
	if cfg.Database.URI != "" {
		if dbErr := storage.InitDB(cfg.Database.URI, cfg.Database.DBName); dbErr != nil {
			log.Printf("警告: 连接 Mongo 失败, 退化为 yaml 静态环 (无法感知在线扩缩容): %v", dbErr)
		} else {
			mongoReady = true
		}
	}

	ringMgr := storage.BuildRingManager(cfg.HomeRing.Nodes, cfg.HomeRing.Version, cfg.HomeRing.PrevNodes, "world")
	if ringMgr == nil {
		log.Printf("警告: Mongo 环文档与 home_ring 配置均为空, world → home 主动通知不可用")
	} else if mongoReady {
		storage.StartRingPoller(ringMgr, 3*time.Second, "world")
	}

	// 6. Spawn 启动大地图统一入口 WorldActor ("world_actor")
	pid, spawnErr := worldNode.SpawnRegister(gen.Atom("world_actor"), func() gen.ProcessBehavior {
		return &world.WorldActor{}
	}, gen.ProcessOptions{}, ringMgr)

	if spawnErr != nil {
		log.Fatalf("派生 WorldActor 失败: %v", spawnErr)
	}
	log.Printf("成功派生 WorldActor: %s", pid.String())

	// 7. 阻塞主线程保持运行
	select {}
}
