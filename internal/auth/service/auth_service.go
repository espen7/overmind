package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"overmind/internal/auth/domain"
)

type authRepository interface {
	FindAccount(username string) (domain.Account, error)
	SaveToken(token string, account domain.Account)
	FindByToken(token string) (domain.Account, error)
}

type AuthService struct {
	repo authRepository
}

type LoginResult struct {
	Token      string
	PlayerID   int64
	PlayerName string
	SceneID    int64
	X          int32
	Y          int32
}

func New(repo authRepository) *AuthService {
	return &AuthService{repo: repo}
}

func (s *AuthService) Login(username string, password string) (LoginResult, error) {
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

func (s *AuthService) Validate(token string) (domain.Account, error) {
	return s.repo.FindByToken(token)
}
