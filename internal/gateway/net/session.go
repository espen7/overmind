package net

type Session struct {
	connID     string
	playerID   int64
	token      string
	playerName string
	sceneID    int64
	x          int32
	y          int32
}

func NewSession(connID string) *Session {
	return &Session{connID: connID}
}

// Bind attaches identity to the transport session after portal login succeeds.
func (s *Session) Bind(playerID int64, token string) {
	s.playerID = playerID
	s.token = token
}

// SetSpawn stores the world-entry context returned by portal for later enter-scene dispatch.
func (s *Session) SetSpawn(playerName string, sceneID int64, x int32, y int32) {
	s.playerName = playerName
	s.sceneID = sceneID
	s.x = x
	s.y = y
}

func (s *Session) PlayerID() int64 {
	return s.playerID
}

func (s *Session) Token() string {
	return s.token
}

func (s *Session) PlayerName() string {
	return s.playerName
}

func (s *Session) SceneID() int64 {
	return s.sceneID
}

func (s *Session) Spawn() (int32, int32) {
	return s.x, s.y
}
