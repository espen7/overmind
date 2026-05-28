package domain

type Scene struct {
	ID int64
}

type EnterResult struct {
	SceneID         int64
	Self            Player
	VisiblePlayers  []Player
	VisibleMonsters []Monster
}

type MoveResult struct {
	SceneID        int64
	Player         Player
	VisiblePlayers []Player
}

type AttackResult struct {
	SceneID    int64
	AttackerID int64
	TargetID   int64
	Damage     int32
	MonsterHP  int32
	Dead       bool
}
