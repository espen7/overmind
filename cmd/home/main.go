package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"

	"overmind/internal/app/home"
	"overmind/internal/pkg/config"
	"overmind/internal/pkg/storage"
)

var (
	configPath = flag.String("config", "configs/home.yaml", "Path to home configuration file")
)

func main() {
	flag.Parse()

	// 1. 载入配置文件
	cfg, err := config.LoadHomeConfig(*configPath)
	if err != nil {
		log.Fatalf("载入逻辑服配置文件失败 [%s]: %v", *configPath, err)
	}

	// 2. 初始化 MongoDB 连接
	err = storage.InitDB(cfg.Database.URI, cfg.Database.DBName)
	if err != nil {
		log.Fatalf("初始化 MongoDB 数据库失败: %v", err)
	}
	log.Printf("MongoDB 连接初始化成功，URI: %s, DB: %s", cfg.Database.URI, cfg.Database.DBName)

	// 3. 启动全局异步存盘服务
	saveWorkers := cfg.Database.SaveWorkers
	if saveWorkers <= 0 {
		saveWorkers = 4
	}
	batchSize := cfg.Database.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}
	storage.InitSaveService(saveWorkers, batchSize)

	// 4. 配置节点选项 (ergo V3)
	opts := gen.NodeOptions{}
	opts.Network.Cookie = cfg.Node.Cookie

	// 配置监听端口
	opts.Network.Acceptors = []gen.AcceptorOptions{
		{
			Host: cfg.Node.ListenHost,
			Port: cfg.Node.ListenPort,
		},
	}

	// 5. 启动 Home 节点
	homeNode, err := ergo.StartNode(gen.Atom(cfg.Node.Name), opts)
	if err != nil {
		log.Fatalf("启动 Home 节点失败: %v", err)
	}
	log.Printf("Home 节点启动成功: %s", homeNode.Name())

	// 6. 节点间寻址无需静态路由: ergo 内嵌 registrar (host:4499) 按节点名自动解析监听端口

	// 7. 构建哈希环管理器: 运行期真相源为 Mongo 环文档 (yaml 播种/兜底), 3s 轮询热切
	// （协调器分配 guard 用当前环自检归属, 释放握手用上一版环反查旧归属）
	ringMgr := storage.BuildRingManager(cfg.HomeRing.Nodes, cfg.HomeRing.Version, cfg.HomeRing.PrevNodes, "home")
	if ringMgr == nil {
		log.Fatalf("配置错误: Mongo 环文档与 home_ring.nodes 均为空, 无法构建哈希环")
	}
	storage.StartRingPoller(ringMgr, 3*time.Second, "home")

	// 8. Spawn 启动单例 HomeCoordinatorActor ("home_coordinator")
	coordinatorName := "home_coordinator"
	pid, spawnErr := homeNode.SpawnRegister(gen.Atom(coordinatorName), func() gen.ProcessBehavior {
		return &home.HomeCoordinatorActor{}
	}, gen.ProcessOptions{}, ringMgr, cfg.WorldNode)

	if spawnErr != nil {
		log.Fatalf("派生 HomeCoordinatorActor 失败: %v", spawnErr)
	}
	log.Printf("成功派生 HomeCoordinatorActor: %s", pid.String())

	// 9. 优雅关闭：监听系统信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	log.Println("收到关闭信号，开始优雅停机...")

	// 停止节点（会触发所有 Actor 的 Terminate）
	homeNode.Stop()

	// 排空存盘队列，等待所有写入完成
	storage.DrainAndClose()

	// 关闭 MongoDB 连接
	storage.CloseDB()

	log.Println("Home 节点已安全关闭")
}
