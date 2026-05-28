package repository

import (
	"fmt"
	"sync"

	"overmind/internal/portal/domain"
)

type MemoryRepository struct {
	mu       sync.RWMutex
	accounts map[string]domain.Account
	tokens   map[string]domain.Account
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		accounts: map[string]domain.Account{
			"demo": {
				Username: "demo",
				Password: "demo",
				PlayerID: 10001,
				Name:     "DemoKnight",
				SceneID:  1,
				X:        120,
				Y:        120,
			},
		},
		tokens: make(map[string]domain.Account),
	}
}

func (r *MemoryRepository) FindAccount(username string) (domain.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, ok := r.accounts[username]
	if !ok {
		return domain.Account{}, fmt.Errorf("account not found")
	}
	return account, nil
}

func (r *MemoryRepository) SaveToken(token string, account domain.Account) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token] = account
}

func (r *MemoryRepository) FindByToken(token string) (domain.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, ok := r.tokens[token]
	if !ok {
		return domain.Account{}, fmt.Errorf("token not found")
	}
	return account, nil
}
