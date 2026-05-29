package messages

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	kitpb "overmind/pkg/pb/kit"
)

const (
	MeshCmdPlayerBind   = "player.bind"
	MeshCmdPlayerUnbind = "player.unbind"
	MeshCmdPlayerError  = "player.error"
)

// gateway 和 player 之间的会话同步统一走 Envelope + protobuf。
// 这样跨进程边界不再泄漏 Go struct，也便于后续替换成真正的 cluster provider。
func NewPlayerBindEnvelope(request *kitpb.PlayerBindRequest) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal player bind request: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdPlayerBind,
				Params:        params,
				SenderService: "gateway",
				TargetService: "player",
			},
		},
	}, nil
}

func DecodePlayerBindEnvelope(envelope *kitpb.Envelope) (*kitpb.PlayerBindRequest, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("player bind envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdPlayerBind {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var request kitpb.PlayerBindRequest
	if err := proto.Unmarshal(mesh.GetParams(), &request); err != nil {
		return nil, fmt.Errorf("unmarshal player bind request: %w", err)
	}
	return &request, nil
}

func NewPlayerUnbindEnvelope(request *kitpb.PlayerUnbindRequest) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal player unbind request: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdPlayerUnbind,
				Params:        params,
				SenderService: "gateway",
				TargetService: "player",
			},
		},
	}, nil
}

func DecodePlayerUnbindEnvelope(envelope *kitpb.Envelope) (*kitpb.PlayerUnbindRequest, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("player unbind envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdPlayerUnbind {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var request kitpb.PlayerUnbindRequest
	if err := proto.Unmarshal(mesh.GetParams(), &request); err != nil {
		return nil, fmt.Errorf("unmarshal player unbind request: %w", err)
	}
	return &request, nil
}

func NewPlayerBindResponseEnvelope(response *kitpb.PlayerBindResponse) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshal player bind response: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdPlayerBind,
				Params:        params,
				SenderService: "player",
				TargetService: "gateway",
			},
		},
	}, nil
}

func DecodePlayerBindResponseEnvelope(envelope *kitpb.Envelope) (*kitpb.PlayerBindResponse, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("player bind response envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdPlayerBind {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var response kitpb.PlayerBindResponse
	if err := proto.Unmarshal(mesh.GetParams(), &response); err != nil {
		return nil, fmt.Errorf("unmarshal player bind response: %w", err)
	}
	return &response, nil
}

func NewPlayerUnbindResponseEnvelope(response *kitpb.PlayerUnbindResponse) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshal player unbind response: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdPlayerUnbind,
				Params:        params,
				SenderService: "player",
				TargetService: "gateway",
			},
		},
	}, nil
}

func DecodePlayerUnbindResponseEnvelope(envelope *kitpb.Envelope) (*kitpb.PlayerUnbindResponse, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("player unbind response envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdPlayerUnbind {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var response kitpb.PlayerUnbindResponse
	if err := proto.Unmarshal(mesh.GetParams(), &response); err != nil {
		return nil, fmt.Errorf("unmarshal player unbind response: %w", err)
	}
	return &response, nil
}

func NewPlayerErrorEnvelope(reason string) *kitpb.Envelope {
	params, err := proto.Marshal(&kitpb.PlayerError{Reason: reason})
	if err != nil {
		params = []byte(reason)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdPlayerError,
				Params:        params,
				SenderService: "player",
				TargetService: "gateway",
			},
		},
	}
}

func DecodePlayerErrorEnvelope(envelope *kitpb.Envelope) error {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return fmt.Errorf("player error envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdPlayerError {
		return fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var response kitpb.PlayerError
	if err := proto.Unmarshal(mesh.GetParams(), &response); err != nil {
		return fmt.Errorf("unmarshal player error: %w", err)
	}
	return fmt.Errorf("%s", response.GetReason())
}
