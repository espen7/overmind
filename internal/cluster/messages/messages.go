package messages

import (
	"fmt"

	"github.com/asynkron/protoactor-go/actor"
)

type IdentityMessage interface {
	Identity() string
}

// WorldLoginReq 是 channelActor 发给 WorldActor 的第一跳登录请求。
// 这一跳只负责把账号映射到 world 内认可的 playerID，并把当前连接上下文带过去。
type WorldLoginReq struct {
	WorldID    int64
	Account    string
	ConnID     string
	ChannelPID *actor.PID
}

func (m WorldLoginReq) Identity() string {
	return fmt.Sprintf("world/%d", m.WorldID)
}

// PlayerLoginReq 是 WorldActor 转发给 PlayerActor 的第二跳登录请求。
// 从这里开始进入“玩家私有数据真源”边界，由 player 节点决定是否接受当前连接。
type PlayerLoginReq struct {
	PlayerID   int64
	WorldID    int64
	Account    string
	ConnID     string
	ChannelPID *actor.PID
}

func (m PlayerLoginReq) Identity() string {
	return fmt.Sprintf("player/%d", m.PlayerID)
}

type PlayerLoginResp struct {
	PlayerID int64
	WorldID  int64
	ConnID   string
}

// PlayerLoginRejected 表示 playerActor 拒绝了本次登录绑定。
type PlayerLoginRejected struct {
	Reason string
}

// WorldLoginRejected 表示 world 阶段就没能继续把登录流程推进到 playerActor。
type WorldLoginRejected struct {
	Reason string
}

// ChannelExpired 用来通知旧连接主动失效，解决同一玩家顶号后的清理问题。
type ChannelExpired struct {
	ConnID string
}
