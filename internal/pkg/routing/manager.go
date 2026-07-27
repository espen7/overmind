package routing

import "sync/atomic"

// Manager 持有当前环与上一版环，支持在线原子切换。
// 网关与 Home 协调器共用：网关用 Current 做路由；协调器用 Previous 反查旧归属发起释放握手。
type Manager struct {
	current  atomic.Pointer[Ring]
	previous atomic.Pointer[Ring]
}

// NewManager 创建环管理器。prevNodes 为空时表示无上一版环（首次部署）
func NewManager(nodes []string, version int, prevNodes []string) *Manager {
	m := &Manager{}
	m.current.Store(New(nodes, version))
	if len(prevNodes) > 0 {
		m.previous.Store(New(prevNodes, version-1))
	}
	return m
}

// Current 返回当前生效的哈希环
func (m *Manager) Current() *Ring {
	return m.current.Load()
}

// Previous 返回上一版哈希环，可能为 nil
func (m *Manager) Previous() *Ring {
	return m.previous.Load()
}

// Update 在线切换到新环：当前环自动降级为上一版环。
// 若新版本号不大于当前版本则忽略（防止乱序/重复推送）
func (m *Manager) Update(nodes []string, version int) bool {
	cur := m.current.Load()
	if cur != nil && version <= cur.Version() {
		return false
	}
	m.previous.Store(cur)
	m.current.Store(New(nodes, version))
	return true
}

// Apply 按环文档全量应用当前环与上一版环（Mongo 轮询热切场景）。
// 与 Update 不同: 上一版环以文档 prev_nodes 为准而非本地降级,
// 保证 commit 清空 prev 后本地不再残留旧环 (避免释放握手反查到已下线节点)。
// 若新版本号不大于当前版本则忽略
func (m *Manager) Apply(nodes []string, version int, prevNodes []string) bool {
	cur := m.current.Load()
	if cur != nil && version <= cur.Version() {
		return false
	}
	m.current.Store(New(nodes, version))
	if len(prevNodes) > 0 {
		m.previous.Store(New(prevNodes, version-1))
	} else {
		m.previous.Store(nil)
	}
	return true
}

// PickCurrent 用当前环定位归属节点
func (m *Manager) PickCurrent(key string) string {
	return m.Current().Pick(key)
}

// PickPrevious 用上一版环定位旧归属节点，无上一版环时返回空串
func (m *Manager) PickPrevious(key string) string {
	prev := m.Previous()
	if prev == nil {
		return ""
	}
	return prev.Pick(key)
}
