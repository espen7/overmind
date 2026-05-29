package domain

import (
	"fmt"
	"time"
)

// Player 是 player 节点持有的玩家主聚合。
// 第一版只先落基础身份、世界归属和登录快照，后面再继续承接建筑、科技、背包等主数据。
type Player struct {
	ID          int64     `bson:"_id"`
	WorldID     int64     `bson:"world_id"`
	Account     string    `bson:"account"`
	Name        string    `bson:"name"`
	Version     int64     `bson:"version"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
	LastLoginAt time.Time `bson:"last_login_at"`
}

func NewPlayer(id int64) Player {
	now := time.Now().UTC()
	return Player{
		ID:        id,
		Name:      fmt.Sprintf("Player-%d", id),
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (p *Player) ApplyLogin(worldID int64, account string) {
	now := time.Now().UTC()

	p.WorldID = worldID
	p.Account = account
	if p.Name == "" {
		p.Name = fmt.Sprintf("Player-%d", p.ID)
	}
	p.LastLoginAt = now
	p.UpdatedAt = now
	p.Version++
}

type PlayerAction struct {
	ID        string    `bson:"_id"`
	PlayerID  int64     `bson:"player_id"`
	NumericID int64     `bson:"numeric_id"`
	ActionID  int32     `bson:"action_id"`
	StartAt   int64     `bson:"start_at"`
	EndAt     int64     `bson:"end_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func NewPlayerAction(playerID int64, numericID int64, actionID int32) PlayerAction {
	return PlayerAction{
		ID:        fmt.Sprintf("%d_%d", playerID, numericID),
		PlayerID:  playerID,
		NumericID: numericID,
		ActionID:  actionID,
		UpdatedAt: time.Now().UTC(),
	}
}
