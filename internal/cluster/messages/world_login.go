package messages

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	kitpb "overmind/pkg/pb/kit"
)

const (
	MeshCmdWorldLogin = "world.login"
)

func NewWorldLoginEnvelope(request *kitpb.WorldLoginRequest) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal world login request: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdWorldLogin,
				Params:        params,
				SenderService: "gateway",
				TargetService: "world",
			},
		},
	}, nil
}

func DecodeWorldLoginEnvelope(envelope *kitpb.Envelope) (*kitpb.WorldLoginRequest, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("world login envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdWorldLogin {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var request kitpb.WorldLoginRequest
	if err := proto.Unmarshal(mesh.GetParams(), &request); err != nil {
		return nil, fmt.Errorf("unmarshal world login request: %w", err)
	}
	return &request, nil
}

func NewWorldLoginResponseEnvelope(response *kitpb.WorldLoginResponse) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshal world login response: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdWorldLogin,
				Params:        params,
				SenderService: "world",
				TargetService: "gateway",
			},
		},
	}, nil
}

func DecodeWorldLoginResponseEnvelope(envelope *kitpb.Envelope) (*kitpb.WorldLoginResponse, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("world login response envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdWorldLogin {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var response kitpb.WorldLoginResponse
	if err := proto.Unmarshal(mesh.GetParams(), &response); err != nil {
		return nil, fmt.Errorf("unmarshal world login response: %w", err)
	}
	return &response, nil
}
