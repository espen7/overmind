package service

import (
	"overmind/internal/world/domain"
	worldpb "overmind/pkg/pb/world"
)

func BuildSnapshot(result domain.EnterResult) *worldpb.SceneSnapshot {
	players := make([]*worldpb.Player, 0, len(result.VisiblePlayers))
	for _, player := range result.VisiblePlayers {
		players = append(players, &worldpb.Player{
			Id:      player.ID,
			Name:    player.Name,
			SceneId: player.SceneID,
			X:       player.X,
			Y:       player.Y,
		})
	}

	monsters := make([]*worldpb.Monster, 0, len(result.VisibleMonsters))
	for _, monster := range result.VisibleMonsters {
		monsters = append(monsters, &worldpb.Monster{
			Id:      monster.ID,
			Name:    monster.Name,
			SceneId: monster.SceneID,
			Hp:      monster.HP,
			MaxHp:   monster.MaxHP,
			X:       monster.X,
			Y:       monster.Y,
			Dead:    monster.Dead,
		})
	}

	return &worldpb.SceneSnapshot{
		SceneId: result.SceneID,
		Self: &worldpb.Player{
			Id:      result.Self.ID,
			Name:    result.Self.Name,
			SceneId: result.Self.SceneID,
			X:       result.Self.X,
			Y:       result.Self.Y,
		},
		Players:  players,
		Monsters: monsters,
	}
}

func BuildMoveBroadcast(result domain.MoveResult) *worldpb.MoveBroadcast {
	return &worldpb.MoveBroadcast{
		PlayerId: result.Player.ID,
		SceneId:  result.SceneID,
		X:        result.Player.X,
		Y:        result.Player.Y,
	}
}

func BuildCombatBroadcast(result domain.AttackResult) *worldpb.CombatBroadcast {
	return &worldpb.CombatBroadcast{
		AttackerId: result.AttackerID,
		TargetId:   result.TargetID,
		SceneId:    result.SceneID,
		Damage:     result.Damage,
		TargetHp:   result.MonsterHP,
		Dead:       result.Dead,
	}
}
