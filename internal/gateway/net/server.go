package net

import (
	"context"
)

// NetworkServer 定义了网关网络服务的通用接口 (策略模式)。
// 具体的实现可以是 WebSocket Server, TCP Server, QUIC Server 等。
type NetworkServer interface {
	// Start 启动网络服务 (非阻塞)
	// ctx: 用于控制生命周期
	Start(ctx context.Context) error

	// Stop 停止网络服务并释放资源
	Stop(ctx context.Context) error

	// Protocol 返回协议名称 (e.g. "ws", "tcp")
	Protocol() string
}
