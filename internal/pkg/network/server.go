package network

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WSServer WebSocket 服务端底座
type WSServer struct {
	addr       string
	upgrader   websocket.Upgrader
	onConnect  func(conn *websocket.Conn) // 新连接建立时的回调函数
	httpServer *http.Server
	wg         sync.WaitGroup
}

// NewWSServer 创建一个新的 WebSocket 服务实例
func NewWSServer(addr string, onConnect func(conn *websocket.Conn)) *WSServer {
	return &WSServer{
		addr: addr,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// 开发调试阶段，允许所有来源的跨域连接
				return true
			},
		},
		onConnect: onConnect,
	}
}

// Start 启动 WebSocket 服务
func (s *WSServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	log.Printf("[WSServer] WebSocket 监听服务正在启动: ws://%s/ws", s.addr)
	return s.httpServer.ListenAndServe()
}

// Stop 停止 WebSocket 服务
func (s *WSServer) Stop() error {
	if s.httpServer != nil {
		log.Printf("[WSServer] 正在关闭 WebSocket 服务...")
		return s.httpServer.Shutdown(context.Background())
	}
	return nil
}

// handleWebSocket 升降级 HTTP 请求并捕获长连接
func (s *WSServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WSServer] HTTP 协议升级为 WebSocket 失败: %v", err)
		return
	}

	log.Printf("[WSServer] 收到来自 %s 的新 WebSocket 长连接", conn.RemoteAddr().String())
	if s.onConnect != nil {
		s.onConnect(conn)
	}
}
