package service

import "overmind/internal/world/domain"

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

// Attack performs the simplest possible scene-local combat resolution for phase one.
func (s *CombatService) Attack(playerID int64, sceneID int64, targetID int64) (domain.AttackResult, error) {
	monster, err := s.world.FindMonster(sceneID, targetID)
	if err != nil {
		return domain.AttackResult{}, err
	}

	// 这里故意保持极简：固定伤害、无技能链、无 Buff、无仇恨。
	// 目标是先验证“世界内战斗事件 -> 广播”这条最短闭环。
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
