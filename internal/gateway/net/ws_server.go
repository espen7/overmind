package net

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"overmind/internal/gateway/protocol"
	"overmind/internal/platform/logging"
	portaltransport "overmind/internal/portal/transport"
	worldtransport "overmind/internal/world/transport"
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
	worldHandler  *worldtransport.Handler
	httpServer    *http.Server
	clients       sync.Map
	players       sync.Map
	nextID        uint64
}

// 这里仍然保留旧的 handler 直调链路，目的是让现有 websocket 能继续跑通。
// 后续 actor 热链路接完后，handlePacket 会逐步切到 channelActor -> world/player。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewWSServer(addr string, portalHandler *portaltransport.Handler, worldHandler *worldtransport.Handler) *WSServer {
	return &WSServer{
		addr:          addr,
		portalHandler: portalHandler,
		worldHandler:  worldHandler,
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

// serveWS owns a single client connection from upgrade to disconnect.
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
		s.clients.Delete(connID)
		if currentClient.session.PlayerID() != 0 {
			s.players.Delete(currentClient.session.PlayerID())
		}
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

// handlePacket keeps the gateway thin: decode, dispatch, and write back.
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
			// 旧链路里仍然由 gateway 直接维护 playerID -> client 的映射，
			// 方便 scene/world 返回广播结果时回写到对应连接。
			currentClient.session.Bind(resp.GetPlayerId(), resp.GetToken())
			currentClient.session.SetSpawn(resp.GetPlayerName(), resp.GetSceneId(), resp.GetX(), resp.GetY())
			s.players.Store(resp.GetPlayerId(), currentClient)
		}
		return s.writeProto(currentClient, protocol.MessageTypeLoginResponse, resp)
	case protocol.MessageTypeEnterScene:
		x, y := currentClient.session.Spawn()
		outbound, err := s.worldHandler.EnterScene(
			currentClient.session.PlayerID(),
			currentClient.session.PlayerName(),
			currentClient.session.SceneID(),
			x,
			y,
		)
		if err != nil {
			return err
		}
		return s.broadcast(outbound)
	case protocol.MessageTypeMoveRequest:
		var req worldpb.MoveRequest
		if err := proto.Unmarshal(packet.Payload, &req); err != nil {
			return err
		}
		outbound, err := s.worldHandler.Move(currentClient.session.PlayerID(), &req)
		if err != nil {
			return err
		}
		return s.broadcast(outbound)
	case protocol.MessageTypeAttackRequest:
		var req worldpb.AttackRequest
		if err := proto.Unmarshal(packet.Payload, &req); err != nil {
			return err
		}
		outbound, err := s.worldHandler.Attack(currentClient.session.PlayerID(), &req)
		if err != nil {
			return err
		}
		return s.broadcast(outbound)
	default:
		return fmt.Errorf("unknown message type %d", packet.Type)
	}
}

// broadcast fans a world event out to every visible player still connected.
func (s *WSServer) broadcast(messages []worldtransport.Outbound) error {
	for _, message := range messages {
		for _, recipient := range message.Recipients {
			// recipients 是 world 侧算好的可见玩家列表，gateway 这里只做纯转发。
			value, ok := s.players.Load(recipient)
			if !ok {
				continue
			}
			target := value.(*client)
			if err := s.writeProto(target, message.Type, message.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *WSServer) writeProto(target *client, messageType uint16, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	packet := protocol.Encode(protocol.Packet{Type: messageType, Payload: payload})
	target.mu.Lock()
	defer target.mu.Unlock()
	return target.conn.WriteMessage(websocket.BinaryMessage, packet)
}
