package channel

import (
	"log"

	"github.com/asynkron/protoactor-go/actor"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"overmind/internal/gateway/protocol"
	logger "overmind/internal/kit/log"
	gatewaypb "overmind/pkg/pb/gateway"
	kitpb "overmind/pkg/pb/kit"
)

// ChannelActor manages the lifecycle of a client connection
type ChannelActor struct {
	sessionID int64
	aesKey    []byte
	writer    chan<- []byte // Channel to send data back to WS Writer Goroutine
}

// NewChannelActor creates the props for a ChannelActor
func NewChannelActor(sessionID int64, writer chan<- []byte) actor.Actor {
	return &ChannelActor{
		sessionID: sessionID,
		writer:    writer,
		// Default Key (mocked for now, should be from config)
		aesKey: []byte("EDb35olv1SRQG5NT"),
	}
}

func (state *ChannelActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case *actor.Started:
		logger.Info("ChannelActor started", zap.Int64("session_id", state.sessionID))

	case *protocol.Packet:
		state.handlePacket(ctx, msg)

	case *kitpb.Envelope:
		// Handle message from backend (EdgeLetter from Game/Portal)
		if edge := msg.GetEdge(); edge != nil {
			state.sendToClient(edge.MsgType, edge.MsgNo, 0, edge.Body)
		}

	case *actor.Stopping:
		logger.Info("ChannelActor stopping", zap.Int64("session_id", state.sessionID))
	}
}

func (state *ChannelActor) handlePacket(ctx actor.Context, packet *protocol.Packet) {
	// TODO: Handle specific MsgTypes (Handshake, Heartbeat) locally
	// Forward others to Portal/Game

	switch gatewaypb.MsgType(packet.MsgType) {
	case gatewaypb.MsgType_HANDSHAKE_REQ:
		state.handleHandshake(packet)
	case gatewaypb.MsgType_HEARTBEAT:
		// Handle heartbeat
	default:
		// Forward to Portal or Game based on login state
		// For now just print
		logger.Info("Received msg", zap.Int32("type", packet.MsgType), zap.Int("len", len(packet.Body)))
	}
}

func (state *ChannelActor) handleHandshake(packet *protocol.Packet) {
	// Mock Handshake Logic
	resp := &gatewaypb.HandshakeResp{
		Rt:         0,
		ServerTime: "123456789",
		SessionKey: []byte("new-session-key"), // In real world this should be encrypted with RSA or predefined
	}

	body, _ := proto.Marshal(resp)

	// Send response
	state.sendToClient(int32(gatewaypb.MsgType_HANDSHAKE_RESP), packet.MsgNo, 0, body)
}

func (state *ChannelActor) sendToClient(msgType int32, msgNo int32, rt int32, body []byte) {
	// Pack the message: Header + Body + CRC
	packed, err := protocol.Pack(msgType, msgNo, rt, body)
	if err != nil {
		logger.Error("Failed to pack message", zap.Error(err))
		return
	}

	// Send to writer goroutine via channel
	select {
	case state.writer <- packed:
	default:
		log.Printf("Client writer channel full, dropping message")
	}
}
