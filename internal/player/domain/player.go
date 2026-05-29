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
