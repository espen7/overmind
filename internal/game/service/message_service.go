package service

import (
	"overmind/internal/game/domain"
	gamepb "overmind/pkg/pb/game"
)

func BuildSnapshot(result domain.EnterResult) *gamepb.SceneSnapshot {
	players := make([]*gamepb.Player, 0, len(result.VisiblePlayers))
	for _, player := range result.VisiblePlayers {
		players = append(players, &gamepb.Player{
			Id:      player.ID,
			Name:    player.Name,
			SceneId: player.SceneID,
			X:       player.X,
			Y:       player.Y,
		})
	}

	monsters := make([]*gamepb.Monster, 0, len(result.VisibleMonsters))
	for _, monster := range result.VisibleMonsters {
		monsters = append(monsters, &gamepb.Monster{
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

	return &gamepb.SceneSnapshot{
		SceneId: result.SceneID,
		Self: &gamepb.Player{
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

func BuildMoveBroadcast(result domain.MoveResult) *gamepb.MoveBroadcast {
	return &gamepb.MoveBroadcast{
		PlayerId: result.Player.ID,
		SceneId:  result.SceneID,
		X:        result.Player.X,
		Y:        result.Player.Y,
	}
}

func BuildCombatBroadcast(result domain.AttackResult) *gamepb.CombatBroadcast {
	return &gamepb.CombatBroadcast{
		AttackerId: result.AttackerID,
		TargetId:   result.TargetID,
		SceneId:    result.SceneID,
		Damage:     result.Damage,
		TargetHp:   result.MonsterHP,
		Dead:       result.Dead,
	}
}
