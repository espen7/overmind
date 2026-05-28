package transport

import (
	"overmind/internal/auth/service"
	authpb "overmind/pkg/pb/auth"
)

type Handler struct {
	service *service.AuthService
}

func NewHandler(s *service.AuthService) *Handler {
	return &Handler{service: s}
}

func (h *Handler) Login(req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	result, err := h.service.Login(req.GetUsername(), req.GetPassword())
	if err != nil {
		return &authpb.LoginResponse{
			ErrorCode:    401,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &authpb.LoginResponse{
		Token:      result.Token,
		PlayerId:   result.PlayerID,
		PlayerName: result.PlayerName,
		SceneId:    result.SceneID,
		X:          result.X,
		Y:          result.Y,
	}, nil
}
