package messages

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	kitpb "overmind/pkg/pb/kit"
)

const (
	MeshCmdWorldRoute    = "world.route"
	MeshCmdWorldDispatch = "world.dispatch"
	MeshCmdWorldError    = "world.error"
)

func NewRouteEnvelope(request *kitpb.WorldRouteRequest) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal world route request: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdWorldRoute,
				Params:        params,
				SenderService: "gateway",
				TargetService: "world",
			},
		},
	}, nil
}

func DecodeRouteEnvelope(envelope *kitpb.Envelope) (*kitpb.WorldRouteRequest, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("route envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdWorldRoute {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var request kitpb.WorldRouteRequest
	if err := proto.Unmarshal(mesh.GetParams(), &request); err != nil {
		return nil, fmt.Errorf("unmarshal world route request: %w", err)
	}
	return &request, nil
}

func NewDispatchEnvelope(batch *kitpb.WorldDispatchBatch) (*kitpb.Envelope, error) {
	params, err := proto.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("marshal world dispatch batch: %w", err)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdWorldDispatch,
				Params:        params,
				SenderService: "world",
				TargetService: "gateway",
			},
		},
	}, nil
}

func DecodeDispatchEnvelope(envelope *kitpb.Envelope) (*kitpb.WorldDispatchBatch, error) {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return nil, fmt.Errorf("dispatch envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdWorldDispatch {
		return nil, fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var batch kitpb.WorldDispatchBatch
	if err := proto.Unmarshal(mesh.GetParams(), &batch); err != nil {
		return nil, fmt.Errorf("unmarshal world dispatch batch: %w", err)
	}
	return &batch, nil
}

func NewErrorEnvelope(reason string) *kitpb.Envelope {
	params, err := proto.Marshal(&kitpb.WorldError{Reason: reason})
	if err != nil {
		params = []byte(reason)
	}

	return &kitpb.Envelope{
		Payload: &kitpb.Envelope_Mesh{
			Mesh: &kitpb.MeshLetter{
				Cmd:           MeshCmdWorldError,
				Params:        params,
				SenderService: "world",
				TargetService: "gateway",
			},
		},
	}
}

func DecodeErrorEnvelope(envelope *kitpb.Envelope) error {
	mesh := envelope.GetMesh()
	if mesh == nil {
		return fmt.Errorf("error envelope missing mesh payload")
	}
	if mesh.GetCmd() != MeshCmdWorldError {
		return fmt.Errorf("unexpected mesh command %q", mesh.GetCmd())
	}

	var reply kitpb.WorldError
	if err := proto.Unmarshal(mesh.GetParams(), &reply); err != nil {
		return fmt.Errorf("unmarshal world error: %w", err)
	}
	return fmt.Errorf("%s", reply.GetReason())
}
