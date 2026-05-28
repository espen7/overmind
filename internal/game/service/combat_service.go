package service

import "overmind/internal/game/domain"

type combatRepository interface {
	FindMonster(sceneID int64, targetID int64) (domain.Monster, error)
	SaveMonster(sceneID int64, monster domain.Monster)
}

type CombatService struct {
	world combatRepository
}

func NewCombat(world combatRepository) *CombatService {
	return &CombatService{world: world}
}

func (s *CombatService) Attack(playerID int64, sceneID int64, targetID int64) (domain.AttackResult, error) {
	monster, err := s.world.FindMonster(sceneID, targetID)
	if err != nil {
		return domain.AttackResult{}, err
	}

	damage := int32(20)
	monster.HP -= damage
	if monster.HP <= 0 {
		monster.HP = 0
		monster.Dead = true
	}
	s.world.SaveMonster(sceneID, monster)

	return domain.AttackResult{
		SceneID:    sceneID,
		AttackerID: playerID,
		TargetID:   monster.ID,
		Damage:     damage,
		MonsterHP:  monster.HP,
		Dead:       monster.Dead,
	}, nil
}
