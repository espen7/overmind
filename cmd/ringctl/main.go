// ringctl 集群哈希环运维工具：对 Mongo 环文档做 CAS 修改，各节点 3s 轮询自动热切。
//
// 用法:
//
//	ringctl [-uri mongodb://127.0.0.1:27017] [-db overmind] <命令> [参数]
//
// 命令:
//
//	show                     查看当前环文档
//	init <node1,node2,...>   首次创建环文档 (通常由节点启动播种, 极少用到)
//	add <node>               扩容: 新节点入环, 旧环存入 prev_nodes 供释放握手反查
//	remove <node>            缩容/摘除宕机节点: 节点出环, 旧环存入 prev_nodes
//	commit                   收敛完成后清空 prev_nodes (确认迁移期结束)
//
// 扩容剧本: 起 home2 进程 → ringctl add home2@x.x.x.x → 等各节点热切+玩家惰性迁移 → ringctl commit
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"overmind/internal/pkg/storage"
)

var (
	uri    = flag.String("uri", "mongodb://127.0.0.1:27017", "MongoDB 连接 URI")
	dbName = flag.String("db", "overmind", "数据库名")
)

// casRetries CAS 冲突时的重读重试次数（并发运维极少见，冲突即重读最新版重算）
const casRetries = 3

func main() {
	log.SetFlags(0)
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
	}

	if err := storage.InitDB(*uri, *dbName); err != nil {
		log.Fatalf("连接 MongoDB 失败: %v", err)
	}
	defer storage.CloseDB()

	var err error
	switch args[0] {
	case "show":
		err = cmdShow()
	case "init":
		if len(args) != 2 {
			usage()
		}
		err = cmdInit(splitNodes(args[1]))
	case "add":
		if len(args) != 2 {
			usage()
		}
		err = mutate(func(doc *storage.RingDoc) ([]string, error) {
			if contains(doc.Nodes, args[1]) {
				return nil, fmt.Errorf("节点 %s 已在环中", args[1])
			}
			return append(append([]string(nil), doc.Nodes...), args[1]), nil
		})
	case "remove":
		if len(args) != 2 {
			usage()
		}
		err = mutate(func(doc *storage.RingDoc) ([]string, error) {
			if !contains(doc.Nodes, args[1]) {
				return nil, fmt.Errorf("节点 %s 不在环中", args[1])
			}
			var kept []string
			for _, n := range doc.Nodes {
				if n != args[1] {
					kept = append(kept, n)
				}
			}
			if len(kept) == 0 {
				return nil, fmt.Errorf("拒绝执行: 移除 %s 后环为空, 所有玩家将无法路由", args[1])
			}
			return kept, nil
		})
	case "commit":
		err = cmdCommit()
	default:
		usage()
	}

	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
}

func cmdShow() error {
	doc, err := storage.LoadRing()
	if err != nil {
		return fmt.Errorf("读取环文档失败 (未创建?): %w", err)
	}
	printDoc(doc)
	return nil
}

func cmdInit(nodes []string) error {
	doc, err := storage.LoadOrSeedRing(nodes, 1, nil)
	if err != nil {
		return err
	}
	if doc.Version != 1 || !sameNodes(doc.Nodes, nodes) {
		log.Printf("环文档已存在, 未做修改 (init 只在文档缺失时生效):")
	} else {
		log.Printf("环文档创建成功:")
	}
	printDoc(doc)
	return nil
}

// mutate 通用改环流程: 读文档 → 计算新节点列表 → CAS 写 (旧环入 prev_nodes), 冲突自动重试
func mutate(compute func(doc *storage.RingDoc) ([]string, error)) error {
	for i := 0; i < casRetries; i++ {
		doc, err := storage.LoadRing()
		if err != nil {
			return fmt.Errorf("读取环文档失败 (未创建? 先执行 init): %w", err)
		}
		if len(doc.PrevNodes) > 0 {
			return fmt.Errorf("上一次环变更尚未 commit (prev_nodes 非空), 请先等迁移收敛后执行 ringctl commit")
		}

		newNodes, err := compute(doc)
		if err != nil {
			return err
		}

		newDoc := storage.RingDoc{
			Version:   doc.Version + 1,
			Nodes:     newNodes,
			PrevNodes: doc.Nodes,
		}
		if err = storage.CASUpdateRing(doc.Version, newDoc); err != nil {
			log.Printf("CAS 冲突, 重试 (%d/%d): %v", i+1, casRetries, err)
			continue
		}

		log.Printf("改环成功 (各节点将在轮询周期内热切, 玩家随后惰性迁移; 收敛后请执行 ringctl commit):")
		newDoc.ID = doc.ID
		printDoc(&newDoc)
		return nil
	}
	return fmt.Errorf("CAS 冲突重试 %d 次仍失败, 请确认是否有并发运维操作", casRetries)
}

func cmdCommit() error {
	for i := 0; i < casRetries; i++ {
		doc, err := storage.LoadRing()
		if err != nil {
			return fmt.Errorf("读取环文档失败: %w", err)
		}
		if len(doc.PrevNodes) == 0 {
			log.Printf("prev_nodes 已为空, 无需 commit")
			printDoc(doc)
			return nil
		}

		newDoc := storage.RingDoc{
			Version:   doc.Version + 1,
			Nodes:     doc.Nodes,
			PrevNodes: nil,
		}
		if err = storage.CASUpdateRing(doc.Version, newDoc); err != nil {
			log.Printf("CAS 冲突, 重试 (%d/%d): %v", i+1, casRetries, err)
			continue
		}

		log.Printf("commit 成功, 迁移过渡期结束:")
		newDoc.ID = doc.ID
		printDoc(&newDoc)
		return nil
	}
	return fmt.Errorf("CAS 冲突重试 %d 次仍失败", casRetries)
}

func printDoc(doc *storage.RingDoc) {
	log.Printf("  version:    %d", doc.Version)
	log.Printf("  nodes:      %v", doc.Nodes)
	log.Printf("  prev_nodes: %v", doc.PrevNodes)
}

func splitNodes(s string) []string {
	var nodes []string
	for _, n := range strings.Split(s, ",") {
		if n = strings.TrimSpace(n); n != "" {
			nodes = append(nodes, n)
		}
	}
	if len(nodes) == 0 {
		usage()
	}
	return nodes
}

func contains(nodes []string, target string) bool {
	for _, n := range nodes {
		if n == target {
			return true
		}
	}
	return false
}

func sameNodes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func usage() {
	fmt.Fprintf(os.Stderr, `用法: ringctl [-uri <mongo_uri>] [-db <db_name>] <命令> [参数]

命令:
  show                     查看当前环文档
  init <node1,node2,...>   首次创建环文档
  add <node>               扩容: 新节点入环 (旧环入 prev_nodes)
  remove <node>            缩容/摘除宕机节点: 节点出环 (旧环入 prev_nodes)
  commit                   迁移收敛后清空 prev_nodes
`)
	os.Exit(2)
}
