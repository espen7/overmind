package net

import (
	"context"
	"fmt"

	"net/http"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/gorilla/websocket"
	"overmind/internal/gateway/channel"
	"overmind/internal/gateway/protocol"
	logger "overmind/internal/kit/log"
)

const (
	// 写超时
	writeWait = 10 * time.Second

	// Pong读取超时
	pongWait = 60 * time.Second

	// Ping发送周期
	pingPeriod = (pongWait * 9) / 10

	// 最大消息大小
	maxMessageSize = 4096
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域 (开发环境)
	},
}

// WSServer 实现了 WebSocket 的 NetworkServer 接口
type WSServer struct {
	addr        string
	actorSystem *actor.ActorSystem
	httpServer  *http.Server
}

// NewWSServer 创建 WebSocket 服务实例
func NewWSServer(addr string, system *actor.ActorSystem) *WSServer {
	return &WSServer{
		addr:        addr,
		actorSystem: system,
	}
}

// Protocol 返回协议名称
func (s *WSServer) Protocol() string {
	return "ws"
}

// Start 启动 HTTP Server
func (s *WSServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.serveWs)

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	logger.Info(fmt.Sprintf("Starting WebSocket Server on %s", s.addr))

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(fmt.Sprintf("WebSocket ListenAndServe failed: %v", err))
		}
	}()
	return nil
}

// Stop 停止 HTTP Server
func (s *WSServer) Stop(ctx context.Context) error {
	logger.Info("Stopping WebSocket Server...")
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// serveWs 处理 WebSocket 请求
func (s *WSServer) serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(fmt.Sprintf("Upgrade failed: %v", err))
		return
	}

	// 创建写通道
	writeChan := make(chan []byte, 256)

	// 生成 Session ID
	sessionID := time.Now().UnixNano()

	// 启动 ChannelActor
	props := actor.PropsFromProducer(func() actor.Actor {
		return channel.NewChannelActor(sessionID, writeChan)
	})

	pid, err := s.actorSystem.Root.SpawnNamed(props, fmt.Sprintf("channel-ws-%d", sessionID))
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to spawn channel actor: %v", err))
		conn.Close()
		return
	}

	// 启动写循环 (Goroutine)
	go s.writePump(conn, writeChan)

	// 启动读循环 (当前 Goroutine)
	s.readPump(conn, pid)
}

// readPump 循环读取 WebSocket 消息
func (s *WSServer) readPump(conn *websocket.Conn, destPID *actor.PID) {
	defer func() {
		s.actorSystem.Root.Stop(destPID)
		conn.Close()
	}()

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error(fmt.Sprintf("WebSocket error: %v", err))
			}
			break
		}

		// 简单的解包/解密逻辑
		// 这里使用固定 Key 演示
		defaultKey := []byte("EDb35olv1SRQG5NT")
		packet, err := protocol.Unpack(message, defaultKey)
		if err != nil {
			logger.Error(fmt.Sprintf("Unpack failed: %v", err))
			continue
		}

		// 发送给 Actor
		s.actorSystem.Root.Send(destPID, packet)
	}
}

// writePump 循环写入 WebSocket 消息
func (s *WSServer) writePump(conn *websocket.Conn, writeChan <-chan []byte) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case message, ok := <-writeChan:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel closed
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
