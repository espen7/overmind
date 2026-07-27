package routing

import (
	"fmt"
	"testing"
)

// TestPickDeterministic 同一 key 在同一环上必须永远命中同一节点（多网关路由一致性的前提）
func TestPickDeterministic(t *testing.T) {
	nodes := []string{"home1@127.0.0.1", "home2@127.0.0.1", "home3@127.0.0.1"}
	r1 := New(nodes, 1)
	r2 := New(nodes, 1)

	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("player_%d", i)
		if r1.Pick(key) != r2.Pick(key) {
			t.Fatalf("key %s 在两个相同配置的环上命中了不同节点", key)
		}
	}
}

// TestBalance 虚拟节点应保证负载大致均匀（各节点承载量在均值 ±30% 内）
func TestBalance(t *testing.T) {
	nodes := []string{"home1@127.0.0.1", "home2@127.0.0.1", "home3@127.0.0.1", "home4@127.0.0.1"}
	r := New(nodes, 1)

	counts := make(map[string]int)
	total := 100000
	for i := 0; i < total; i++ {
		counts[r.Pick(fmt.Sprintf("player_%d", i))]++
	}

	avg := total / len(nodes)
	for node, c := range counts {
		if c < avg*70/100 || c > avg*130/100 {
			t.Errorf("节点 %s 负载失衡: %d (均值 %d)", node, c, avg)
		}
	}
}

// TestMinimalRemap 扩容一个节点时，归属变化的 key 应接近 1/N 而非全量重排
func TestMinimalRemap(t *testing.T) {
	oldNodes := []string{"home1@127.0.0.1", "home2@127.0.0.1", "home3@127.0.0.1"}
	newNodes := append(oldNodes, "home4@127.0.0.1")
	oldRing := New(oldNodes, 1)
	newRing := New(newNodes, 2)

	total := 100000
	moved := 0
	for i := 0; i < total; i++ {
		key := fmt.Sprintf("player_%d", i)
		if oldRing.Pick(key) != newRing.Pick(key) {
			moved++
			// 迁移的 key 必须只会迁往新节点，不允许在旧节点之间乱窜
			if newRing.Pick(key) != "home4@127.0.0.1" {
				t.Fatalf("key %s 从 %s 迁往了旧节点 %s", key, oldRing.Pick(key), newRing.Pick(key))
			}
		}
	}

	// 理论迁移比例为 1/4 = 25%，允许 15% ~ 35% 的浮动
	ratio := float64(moved) / float64(total)
	if ratio < 0.15 || ratio > 0.35 {
		t.Errorf("扩容迁移比例异常: %.2f%% (理论值约 25%%)", ratio*100)
	}
	t.Logf("3 -> 4 节点扩容, 迁移比例: %.2f%%", ratio*100)
}

// TestManagerUpdate 换环后 Previous 应为旧环，且拒绝过期版本
func TestManagerUpdate(t *testing.T) {
	mgr := NewManager([]string{"home1@127.0.0.1"}, 1, nil)

	if !mgr.Update([]string{"home1@127.0.0.1", "home2@127.0.0.1"}, 2) {
		t.Fatal("合法的版本 2 换环被拒绝")
	}
	if mgr.Current().Version() != 2 {
		t.Fatalf("当前环版本应为 2, 实际: %d", mgr.Current().Version())
	}
	if mgr.Previous() == nil || mgr.Previous().Version() != 1 {
		t.Fatal("换环后上一版环应为版本 1")
	}

	// 过期/重复版本必须被拒绝
	if mgr.Update([]string{"home9@127.0.0.1"}, 2) {
		t.Fatal("重复版本 2 未被拒绝")
	}
	if mgr.Update([]string{"home9@127.0.0.1"}, 1) {
		t.Fatal("过期版本 1 未被拒绝")
	}
}
