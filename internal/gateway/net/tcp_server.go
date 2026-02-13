package net

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"overmind/internal/gateway/channel"
	"overmind/internal/gateway/protocol"
	logger "overmind/internal/kit/log"
)

// TCPServer 实现了 TCP 的 NetworkServer 接口
// 采用 Length-Prefixed 协议: [Length (2 bytes)] + [Body]
type TCPServer struct {
	addr        string
	actorSystem *actor.ActorSystem
	listener    net.Listener
}

// NewTCPServer 创建 TCP 服务实例
func NewTCPServer(addr string, system *actor.ActorSystem) *TCPServer {
	return &TCPServer{
		addr:        addr,
		actorSystem: system,
	}
}

// Protocol 返回协议名称
func (s *TCPServer) Protocol() string {
	return "tcp"
}

// Start 启动 TCP Listener
func (s *TCPServer) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = ln

	logger.Info(fmt.Sprintf("Starting TCP Server on %s", s.addr))

	go s.acceptLoop()
	return nil
}

// Stop 停止 TCP Listener
func (s *TCPServer) Stop(ctx context.Context) error {
	logger.Info("Stopping TCP Server...")
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// acceptLoop 循环接受连接
func (s *TCPServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// 如果是关闭错误，则退出
			select {
			case <-time.After(0):
				// 检查是否主动关闭，这里简单处理
				logger.Error(fmt.Sprintf("Accept error: %v", err))
			}
			return
		}

		go s.handleConn(conn)
	}
}

// handleConn 处理单个 TCP 连接
func (s *TCPServer) handleConn(conn net.Conn) {
	// 生成 Session ID
	sessionID := time.Now().UnixNano()

	// 创建写通道
	// TCP 写操作可能阻塞，这里也是异步写入
	writeChan := make(chan []byte, 256)

	// 启动 ChannelActor
	props := actor.PropsFromProducer(func() actor.Actor {
		return channel.NewChannelActor(sessionID, writeChan)
	})

	pid, err := s.actorSystem.Root.SpawnNamed(props, fmt.Sprintf("channel-tcp-%d", sessionID))
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to spawn channel actor for TCP: %v", err))
		conn.Close()
		return
	}

	// 启动写循环
	go s.writeLoop(conn, writeChan)

	// 启动读循环
	s.readLoop(conn, pid)
}

// readLoop 读取 TCP 数据流 (Length-Prefixed)
func (s *TCPServer) readLoop(conn net.Conn, destPID *actor.PID) {
	defer func() {
		s.actorSystem.Root.Stop(destPID)
		conn.Close()
	}()

	header := make([]byte, 2) // 2字节长度头

	for {
		// 1. 读取长度
		if _, err := io.ReadFull(conn, header); err != nil {
			if err != io.EOF {
				logger.Error(fmt.Sprintf("TCP Read Header error: %v", err))
			}
			return
		}

		length := binary.BigEndian.Uint16(header)

		// 简单限制最大包体，防止 OOM
		if length > 4096 {
			logger.Error(fmt.Sprintf("TCP Packet too large: %d", length))
			return
		}

		// 2. 读取包体
		body := make([]byte, length)
		if _, err := io.ReadFull(conn, body); err != nil {
			logger.Error(fmt.Sprintf("TCP Read Body error: %v", err))
			return
		}

		// 3. 解包/解密
		defaultKey := []byte("EDb35olv1SRQG5NT")
		packet, err := protocol.Unpack(body, defaultKey)
		if err != nil {
			logger.Error(fmt.Sprintf("Unpack failed: %v", err))
			continue
		}

		// 4. 发送给 Actor
		s.actorSystem.Root.Send(destPID, packet)
	}
}

// writeLoop 写入 TCP 数据
func (s *TCPServer) writeLoop(conn net.Conn, writeChan <-chan []byte) {
	// TCP 不需要 Ping/Pong (通常由应用层心跳处理)，或者可以使用 TCP KeepAlive
	// 这里简单实现只负责发包

	defer func() {
		conn.Close()
	}()

	for msg := range writeChan {
		// 封装 Length-Prefix
		length := uint16(len(msg))
		header := make([]byte, 2)
		binary.BigEndian.PutUint16(header, length)

		// Write Header
		if _, err := conn.Write(header); err != nil {
			return
		}

		// Write Body
		if _, err := conn.Write(msg); err != nil {
			return
		}
	}
}
