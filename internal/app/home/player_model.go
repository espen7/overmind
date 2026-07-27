package home

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

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

// Load 从 MongoDB 加载玩家数据，如果不存在则创建默认数据并写入
// 返回 true 表示是新建玩家。
// 注意：由于 Coordinator 在 Spawn 前会先 AcquireOwnership (upsert 预创建仅含
// _id/epoch/owner_node 的文档)，文档存在但缺少 profile 分区同样视为新玩家。
func (m *PlayerModel) Load() (isNew bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var doc bson.M
	err = storage.PlayerCol.FindOne(ctx, bson.M{"_id": m.PlayerID}).Decode(&doc)

	if err == mongo.ErrNoDocuments || (err == nil && doc["profile"] == nil) {
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

	// 用 $set + upsert 而非 InsertOne：文档可能已由 AcquireOwnership 预创建，
	// 不能覆盖其中的 epoch/owner_node 围栏字段
	update := bson.M{
		"$set": bson.M{
			"profile":    profileDoc,
			"bag":        bagDoc,
			"builds":     buildsDoc,
			"created_at": time.Now(),
			"updated_at": time.Now(),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	opts := options.UpdateOne().SetUpsert(true)
	_, err := storage.PlayerCol.UpdateOne(ctx, bson.M{"_id": m.PlayerID}, update, opts)
	if err != nil {
		log.Printf("[PlayerModel] 创建新玩家档案失败: %v", err)
	}
	return err
}

func (m *PlayerModel) loadFromDoc(doc bson.M) {
	for _, sec := range m.sections {
		if subDoc, ok := doc[sec.Key()]; ok {
			// 用标准 json.Marshal 而非 bson.MarshalExtJSON：
			// - canonical ExtJSON 会把数字包成 {"$numberInt":"400"}
			// - relaxed ExtJSON 会把 double 渲染成 "400.0"
			// 两者都会让 section 的标准 json.Unmarshal 解析 int32 字段失败、数据归零；
			// 而 encoding/json 对整数值的 float64 输出 "400"，可正常解进 int32
			jsonBytes, err := json.Marshal(subDoc)
			if err != nil {
				log.Printf("[PlayerModel] 玩家 %s 分区 %s 序列化失败: %v", m.PlayerID, sec.Key(), err)
				continue
			}
			if err := sec.Unmarshal(jsonBytes); err != nil {
				log.Printf("[PlayerModel] 玩家 %s 分区 %s 反序列化失败: %v", m.PlayerID, sec.Key(), err)
			}
		}
	}
}
