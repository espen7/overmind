package home

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"overmind/internal/pkg/storage"
)

// PlayerModel 对应 MongoDB 中一个完整的玩家文档，由 PlayerActor 在 Init 时加载
type PlayerModel struct {
	PlayerID string

	// 各功能数据分区
	Profile ProfileSection
	Bag     BagSection
	Builds  BuildsSection

	// 所有 Section 的统一引用（用于遍历 flush/load）
	sections []storage.Section
}

// NewPlayerModel 创建并初始化 PlayerModel
func NewPlayerModel(playerID string) *PlayerModel {
	m := &PlayerModel{PlayerID: playerID}
	m.sections = []storage.Section{&m.Profile, &m.Bag, &m.Builds}
	return m
}

// Sections 返回所有 section 列表
func (m *PlayerModel) Sections() []storage.Section {
	return m.sections
}

// Load 从 MongoDB 加载玩家数据，如果不存在则创建默认数据并插入
// 返回 true 表示是新建玩家
func (m *PlayerModel) Load() (isNew bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var doc bson.M
	err = storage.PlayerCol.FindOne(ctx, bson.M{"_id": m.PlayerID}).Decode(&doc)

	if err == mongo.ErrNoDocuments {
		// 新玩家，填充默认数据
		m.initDefaults()

		if insertErr := m.insertNew(); insertErr != nil {
			return false, insertErr
		}

		// 落盘完成，清除脏标记
		m.ClearAllDirty()
		return true, nil
	}
	if err != nil {
		return false, err
	}

	// 已有数据，从文档反序列化
	m.loadFromDoc(doc)
	return false, nil
}

// FlushDirty 收集所有脏字段，返回 MongoDB $set map（"section_key.field_key" -> value）
// 同时清除脏标记
func (m *PlayerModel) FlushDirty() map[string]interface{} {
	dirtyFields := make(map[string]interface{})
	for _, sec := range m.sections {
		if sec.IsDirty() {
			for field, value := range sec.DirtyFields() {
				dirtyFields[sec.Key()+"."+field] = value
			}
			sec.ClearDirty()
		}
	}
	return dirtyFields
}

// ForceFlushAll 强制收集所有字段（Terminate 时确保完整性）
func (m *PlayerModel) ForceFlushAll() map[string]interface{} {
	dirtyFields := make(map[string]interface{})
	for _, sec := range m.sections {
		// 先收集已脏字段
		for field, value := range sec.DirtyFields() {
			dirtyFields[sec.Key()+"."+field] = value
		}
		// 对不脏的模块也全量写入
		if !sec.IsDirty() {
			data, err := sec.Marshal()
			if err == nil {
				var parsed map[string]interface{}
				if json.Unmarshal(data, &parsed) == nil {
					for field, value := range parsed {
						dirtyFields[sec.Key()+"."+field] = value
					}
				}
			}
		}
		sec.ClearDirty()
	}
	return dirtyFields
}

// ClearAllDirty 清除所有分区的脏标记
func (m *PlayerModel) ClearAllDirty() {
	for _, sec := range m.sections {
		sec.ClearDirty()
	}
}

// ==================== 内部方法 ====================

func (m *PlayerModel) initDefaults() {
	m.Profile.SetName("Player_" + m.PlayerID)
	m.Profile.SetGold(500)
	m.Profile.SetPower(1000)
	m.Profile.SetBuildLevel(1)
	m.Bag.SetItemsKey(1001, 50)
	m.Builds.SetBuilds(make(map[string]int32))
}

func (m *PlayerModel) insertNew() error {
	profileData, _ := m.Profile.Marshal()
	bagData, _ := m.Bag.Marshal()
	buildsData, _ := m.Builds.Marshal()

	var profileDoc, bagDoc, buildsDoc bson.M
	_ = bson.UnmarshalExtJSON(profileData, true, &profileDoc)
	_ = bson.UnmarshalExtJSON(bagData, true, &bagDoc)
	_ = bson.UnmarshalExtJSON(buildsData, true, &buildsDoc)

	newDoc := bson.M{
		"_id":        m.PlayerID,
		"profile":    profileDoc,
		"bag":        bagDoc,
		"builds":     buildsDoc,
		"created_at": time.Now(),
		"updated_at": time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := storage.PlayerCol.InsertOne(ctx, newDoc)
	if err != nil {
		log.Printf("[PlayerModel] 创建新玩家档案失败: %v", err)
	}
	return err
}

func (m *PlayerModel) loadFromDoc(doc bson.M) {
	for _, sec := range m.sections {
		if subDoc, ok := doc[sec.Key()]; ok {
			if jsonBytes, err := bson.MarshalExtJSON(subDoc, true, false); err == nil {
				_ = sec.Unmarshal(jsonBytes)
			}
		}
	}
}
