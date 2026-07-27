package routing

import (
	"fmt"
	"hash/crc32"
	"sort"
)

const (
	// VirtualNodes 每个物理节点在哈希环上的虚拟节点数量，保证负载均匀
	VirtualNodes = 150
)

// Ring 一致性哈希环（不可变结构，构建后只读，天然并发安全）
type Ring struct {
	version int      // 环版本号，用于网关识别环变更
	nodes   []string // 物理节点列表 (如 "home1@127.0.0.1")
	keys    []uint32 // 排序后的虚拟节点哈希值
	vnodes  map[uint32]string
}

// New 根据物理节点列表构建一致性哈希环
func New(nodes []string, version int) *Ring {
	r := &Ring{
		version: version,
		nodes:   append([]string(nil), nodes...),
		vnodes:  make(map[uint32]string, len(nodes)*VirtualNodes),
	}

	for _, node := range nodes {
		for i := 0; i < VirtualNodes; i++ {
			h := hashKey(fmt.Sprintf("%s#%d", node, i))
			// 哈希冲突时先到先得，冲突概率极低且所有进程计算结果一致
			if _, exists := r.vnodes[h]; exists {
				continue
			}
			r.vnodes[h] = node
			r.keys = append(r.keys, h)
		}
	}

	sort.Slice(r.keys, func(i, j int) bool { return r.keys[i] < r.keys[j] })
	return r
}

// Pick 根据 key (如 PlayerID) 顺时针定位归属的物理节点
func (r *Ring) Pick(key string) string {
	if len(r.keys) == 0 {
		return ""
	}

	h := hashKey(key)
	// 二分查找第一个 >= h 的虚拟节点，找不到则回绕到环首
	idx := sort.Search(len(r.keys), func(i int) bool { return r.keys[i] >= h })
	if idx == len(r.keys) {
		idx = 0
	}
	return r.vnodes[r.keys[idx]]
}

// Version 返回环版本号
func (r *Ring) Version() int {
	return r.version
}

// Nodes 返回物理节点列表
func (r *Ring) Nodes() []string {
	return r.nodes
}

func hashKey(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}
