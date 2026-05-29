package messages

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	kitpb "overmind/pkg/pb/kit"
)

const (
	MeshCmdPlayerLogin = "player.login"
)

func NewPlayerLoginEnvelope(request *kitpb.PlayerLoginRequest) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal player login request: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdPlayerLogin,
				Params:        params,
				SenderService: "world",
				TargetService: "player",
			},
		},
	}, nil
}

func DecodePlayerLoginEnvelope(envelope *kitpb.Envelope) (*kitpb.PlayerLoginRequest, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("player login envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdPlayerLogin {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var request kitpb.PlayerLoginRequest
	if err := proto.Unmarshal(mesh.GetParams(), &request); err != nil {
		return nil, fmt.Errorf("unmarshal player login request: %w", err)
	}
	return &request, nil
}

func NewPlayerLoginResponseEnvelope(response *kitpb.PlayerLoginResponse) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshal player login response: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdPlayerLogin,
				Params:        params,
				SenderService: "player",
				TargetService: "world",
			},
		},
	}, nil
}

func DecodePlayerLoginResponseEnvelope(envelope *kitpb.Envelope) (*kitpb.PlayerLoginResponse, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("player login response envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdPlayerLogin {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var response kitpb.PlayerLoginResponse
	if err := proto.Unmarshal(mesh.GetParams(), &response); err != nil {
		return nil, fmt.Errorf("unmarshal player login response: %w", err)
	}
	return &response, nil
}
