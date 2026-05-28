package transport

import (
	"overmind/internal/portal/service"
	portalpb "overmind/pkg/pb/portal"
)

type Handler struct {
	service *service.PortalService
}

func NewHandler(s *service.PortalService) *Handler {
	return &Handler{service: s}
}

func (h *Handler) Login(req *portalpb.LoginRequest) (*portalpb.LoginResponse, error) {
	result, err := h.service.Login(req.GetUsername(), req.GetPassword())
	if err != nil {
		return &portalpb.LoginResponse{
			ErrorCode:    401,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &portalpb.LoginResponse{
		Token:      result.Token,
		PlayerId:   result.PlayerID,
		PlayerName: result.PlayerName,
		SceneId:    result.SceneID,
		X:          result.X,
		Y:          result.Y,
	}, nil
}
