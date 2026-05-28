package service

import "sync"

type LoginService interface {
	ResolvePlayer(worldID int64, account string) (playerID int64, created bool, err error)
}

type InMemoryLoginService struct {
	mu        sync.Mutex
	nextID    int64
	playerIDs map[string]int64
}

func NewInMemoryLoginService() *InMemoryLoginService {
	return &InMemoryLoginService{
		nextID:    1000,
		playerIDs: make(map[string]int64),
	}
}

func (s *InMemoryLoginService) ResolvePlayer(worldID int64, account string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if playerID, ok := s.playerIDs[account]; ok {
		return playerID, false, nil
	}

	s.nextID++
	s.playerIDs[account] = s.nextID
	return s.nextID, true, nil
}
