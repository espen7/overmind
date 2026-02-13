package portal

import (
	"log"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"google.golang.org/protobuf/proto"
	gatewaypb "overmind/pkg/pb/gateway"
	kitpb "overmind/pkg/pb/kit"
)

// PortalActor handles initial user entry: Login, Registration, Server Selection.
type PortalActor struct {
	// redis *infra.RedisClient
}

func NewPortalActor() actor.Actor {
	return &PortalActor{
		// redis: redis,
	}
}

func (state *PortalActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		log.Println("PortalActor started")

	case *kitpb.Envelope:
		// Handle Edge/Mesh letters
		if edge := msg.GetEdge(); edge != nil {
			state.handleEdgeLetter(context, msg, edge)
		}

	case *actor.Stopping:
		log.Println("PortalActor stopping")
	}
}

func (state *PortalActor) handleEdgeLetter(ctx actor.Context, envelope *kitpb.Envelope, letter *kitpb.EdgeLetter) {
	senderInfo := ctx.Sender()

	switch gatewaypb.MsgType(letter.MsgType) {
	case gatewaypb.MsgType_LOGIN_REQ:
		// Decode Body
		req := &gatewaypb.LoginReq{}
		if err := proto.Unmarshal(letter.Body, req); err != nil {
			log.Printf("Failed to unmarshal LoginReq: %v", err)
			return
		}

		log.Printf("Processing Login for AppID: %s", req.AppId)

		// Mock Auth Logic - Store session in Redis
		// In real app: Check DB/Redis
		// For now, let's just write to Redis to prove integration
		/*
		   if state.redis != nil {
		        err := state.redis.Client.Set(context.Background(), "session:"+req.AppId, "logged_in", 10*time.Minute).Err()
		        if err != nil {
		            log.Printf("Redis error: %v", err)
		        }
		   }
		*/

		// Construct Response
		resp := &gatewaypb.LoginResp{
			Rt:       0,
			Token:    "mock-token-123456",
			PlayerId: "player-" + req.AppId,
		}
		respBody, _ := proto.Marshal(resp)

		// Reply
		replyLetter := &kitpb.EdgeLetter{
			MsgType:   int32(gatewaypb.MsgType_LOGIN_RESP),
			Body:      respBody,
			SessionId: letter.SessionId,
			MsgNo:     letter.MsgNo,
		}

		replyEnvelope := &kitpb.Envelope{
			Timestamp: time.Now().UnixNano(),
			TraceId:   envelope.TraceId,
			Payload: &kitpb.Envelope_Edge{
				Edge: replyLetter,
			},
		}

		if senderInfo != nil {
			ctx.Send(senderInfo, replyEnvelope)
		}
	}
}
