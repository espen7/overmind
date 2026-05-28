package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"overmind/internal/portal/domain"
)

type authRepository interface {
	FindAccount(username string) (domain.Account, error)
	SaveToken(token string, account domain.Account)
	FindByToken(token string) (domain.Account, error)
}

type LoginResult struct {
	Token      string
	PlayerID   int64
	PlayerName string
	SceneID    int64
	X          int32
	Y          int32
}

type PortalService struct {
	repo authRepository
}

func New(repo authRepository) *PortalService {
	return &PortalService{repo: repo}
}

func (s *PortalService) Login(username string, password string) (LoginResult, error) {
	account, err := s.repo.FindAccount(username)
	if err != nil {
		return LoginResult{}, err
	}
	if account.Password != password {
		return LoginResult{}, fmt.Errorf("invalid credentials")
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return LoginResult{}, fmt.Errorf("generate token: %w", err)
	}

	token := hex.EncodeToString(tokenBytes)
	s.repo.SaveToken(token, account)

	return LoginResult{
		Token:      token,
		PlayerID:   account.PlayerID,
		PlayerName: account.Name,
		SceneID:    account.SceneID,
		X:          account.X,
		Y:          account.Y,
	}, nil
}

func (s *PortalService) Validate(token string) (domain.Account, error) {
	return s.repo.FindByToken(token)
}
