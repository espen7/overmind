package net

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"overmind/internal/gateway/playerproxy"
	"overmind/internal/gateway/protocol"
	"overmind/internal/gateway/worldproxy"
	"overmind/internal/platform/logging"
	portaltransport "overmind/internal/portal/transport"
	portalpb "overmind/pkg/pb/portal"
	worldpb "overmind/pkg/pb/world"
)

type client struct {
	conn    *websocket.Conn
	session *Session
	mu      sync.Mutex
}

type WSServer struct {
	addr          string
	portalHandler *portaltransport.Handler
	playerProxy   *playerproxy.Proxy
	worldProxy    *worldproxy.Proxy
	httpServer    *http.Server
	clients       sync.Map
	players       sync.Map
	nextID        uint64
}

// gateway 现在只承担边缘接入职责:
// 1. 负责 websocket 连接和协议编解码
// 2. 登录时调用 portal 做认证
// 3. 登录成功后调用 player 做在线会话绑定
// 4. gameplay 消息再按领域路由到 world
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewWSServer(
	addr string,
	portalHandler *portaltransport.Handler,
	playerProxy *playerproxy.Proxy,
	worldProxy *worldproxy.Proxy,
) *WSServer {
	return &WSServer{
		addr:          addr,
		portalHandler: portalHandler,
		playerProxy:   playerProxy,
		worldProxy:    worldProxy,
	}
}

func (s *WSServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.serveWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gateway ok"))
	})

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.L().Error("gateway listen failed", logging.Error(err))
		}
	}()

	return nil
}

func (s *WSServer) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// serveWS 管理单条连接从升级到断开的完整生命周期。
func (s *WSServer) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	connID := fmt.Sprintf("conn-%d", atomic.AddUint64(&s.nextID, 1))
	currentClient := &client{
		conn:    conn,
		session: NewSession(connID),
	}
	s.clients.Store(connID, currentClient)
	defer func() {
		if playerID := currentClient.session.PlayerID(); playerID != 0 {
			if err := s.playerProxy.UnbindSession(playerID, currentClient.session.ConnID()); err != nil {
				logging.L().Warn(
					"unbind player session failed",
					logging.Error(err),
					logging.String("conn_id", currentClient.session.ConnID()),
					logging.Int64("player_id", playerID),
				)
			}
			s.players.Delete(playerID)
		}
		s.clients.Delete(connID)
		_ = conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}

		packet, err := protocol.Decode(message)
		if err != nil {
			logging.L().Warn(
				"decode packet failed",
				logging.Error(err),
				logging.String("conn_id", currentClient.session.ConnID()),
			)
			continue
		}
		if err := s.handlePacket(currentClient, packet); err != nil {
			logging.L().Warn(
				"handle packet failed",
				logging.Error(err),
				logging.String("conn_id", currentClient.session.ConnID()),
			)
			_ = s.writeProto(currentClient, protocol.MessageTypeErrorResponse, &worldpb.ErrorResponse{
				ErrorCode:    500,
				ErrorMessage: err.Error(),
			})
		}
	}
}

// handlePacket 保持 gateway 只做薄分发:
// 登录归 portal + player，世界行为归 world。
func (s *WSServer) handlePacket(currentClient *client, packet protocol.Packet) error {
	switch packet.Type {
	case protocol.MessageTypeLoginRequest:
		var req portalpb.LoginRequest
		if err := proto.Unmarshal(packet.Payload, &req); err != nil {
			return err
		}

		resp, err := s.portalHandler.Login(&req)
		if err != nil {
			return err
		}
		if resp.GetErrorCode() == 0 {
			bindResp, err := s.playerProxy.BindSession(playerproxy.BindInput{
				PlayerID: resp.GetPlayerId(),
				WorldID:  1,
				Account:  req.GetUsername(),
				ConnID:   currentClient.session.ConnID(),
			})
			if err != nil {
				return err
			}
			if expiredConnID := bindResp.GetExpiredConnId(); expiredConnID != "" && expiredConnID != currentClient.session.ConnID() {
				// 顶号场景下由 player 节点告知旧连接是谁，gateway 只负责把那条连接真正断开。
				s.expireConn(expiredConnID)
			}

			currentClient.session.Bind(resp.GetPlayerId(), resp.GetToken())
			currentClient.session.SetSpawn(resp.GetPlayerName(), resp.GetSceneId(), resp.GetX(), resp.GetY())
			s.players.Store(resp.GetPlayerId(), currentClient)
		}
		return s.writeProto(currentClient, protocol.MessageTypeLoginResponse, resp)

	case protocol.MessageTypeEnterScene:
		x, y := currentClient.session.Spawn()
		outbound, err := s.worldProxy.EnterScene(worldproxy.EnterSceneInput{
			PlayerID:   currentClient.session.PlayerID(),
			PlayerName: currentClient.session.PlayerName(),
			SceneID:    currentClient.session.SceneID(),
			X:          x,
			Y:          y,
		})
		if err != nil {
			return err
		}
		return s.broadcast(outbound)

	case protocol.MessageTypeMoveRequest:
		var req worldpb.MoveRequest
		if err := proto.Unmarshal(packet.Payload, &req); err != nil {
			return err
		}
		outbound, err := s.worldProxy.Move(currentClient.session.PlayerID(), &req)
		if err != nil {
			return err
		}
		return s.broadcast(outbound)

	case protocol.MessageTypeAttackRequest:
		var req worldpb.AttackRequest
		if err := proto.Unmarshal(packet.Payload, &req); err != nil {
			return err
		}
		outbound, err := s.worldProxy.Attack(currentClient.session.PlayerID(), &req)
		if err != nil {
			return err
		}
		return s.broadcast(outbound)

	default:
		return fmt.Errorf("unknown message type %d", packet.Type)
	}
}

// broadcast 把 world 计算出的投递结果回写给当前仍在线的连接。
func (s *WSServer) broadcast(messages []worldproxy.Outbound) error {
	for _, message := range messages {
		for _, recipient := range message.Recipients {
			value, ok := s.players.Load(recipient)
			if !ok {
				continue
			}

			target := value.(*client)
			if err := s.writePayload(target, message.Type, message.Payload); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *WSServer) expireConn(connID string) {
	value, ok := s.clients.Load(connID)
	if !ok {
		return
	}

	target := value.(*client)
	target.mu.Lock()
	defer target.mu.Unlock()
	_ = target.conn.Close()
}

func (s *WSServer) writeProto(target *client, messageType uint16, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	return s.writePayload(target, messageType, payload)
}

func (s *WSServer) writePayload(target *client, messageType uint16, payload []byte) error {
	packet := protocol.Encode(protocol.Packet{Type: messageType, Payload: payload})
	target.mu.Lock()
	defer target.mu.Unlock()
	return target.conn.WriteMessage(websocket.BinaryMessage, packet)
}
