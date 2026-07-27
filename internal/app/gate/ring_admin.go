package gate

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"overmind/internal/pkg/routing"
)

// RingAdminActor 网关在线换环管理入口 (注册名: gate_ring_admin)。
// 运维工具/管理节点通过跨节点 Call 推送新环，实现不停服扩缩容：
//
//	请求格式: "ring_update <version> <node1,node2,...>"
//	示例:     "ring_update 2 home1@127.0.0.1,home2@127.0.0.1"
//
// 环切换后由各 ChannelActor 在下一个业务包到达时惰性完成玩家迁移。
type RingAdminActor struct {
	act.Actor
	ringMgr *routing.Manager
}

// Init 初始化环管理 Actor
func (ra *RingAdminActor) Init(args ...any) error {
	if len(args) < 1 {
		return fmt.Errorf("bad_arguments")
	}
	mgr, ok := args[0].(*routing.Manager)
	if !ok {
		return fmt.Errorf("bad_arguments")
	}
	ra.ringMgr = mgr
	log.Printf("[RingAdminActor] 换环管理入口启动成功 (当前环版本: %d, 节点: %v)",
		mgr.Current().Version(), mgr.Current().Nodes())
	return nil
}

// HandleCall 处理换环指令，同步返回执行结果
func (ra *RingAdminActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	cmd, ok := request.(string)
	if !ok {
		return "error: invalid_request_type", nil
	}

	fields := strings.Fields(cmd)
	if len(fields) != 3 || fields[0] != "ring_update" {
		return "error: usage: ring_update <version> <node1,node2,...>", nil
	}

	version, err := strconv.Atoi(fields[1])
	if err != nil {
		return "error: invalid_version", nil
	}

	nodes := strings.Split(fields[2], ",")
	if len(nodes) == 0 {
		return "error: empty_nodes", nil
	}

	if !ra.ringMgr.Update(nodes, version) {
		log.Printf("[RingAdminActor] 拒绝换环: 版本 %d 不大于当前版本 %d", version, ra.ringMgr.Current().Version())
		return "error: stale_version", nil
	}

	log.Printf("[RingAdminActor] 在线换环成功: 版本 %d, 节点: %v (归属变化的在线玩家将惰性迁移)", version, nodes)
	return "ok", nil
}
