package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// NodeConfig 描述 Ergo 节点的基本配置
type NodeConfig struct {
	Name       string `yaml:"name"`
	Cookie     string `yaml:"cookie"`
	ListenHost string `yaml:"listen_host"`
	ListenPort uint16 `yaml:"listen_port"`
}

// MongoConfig Mongo 连接配置 (gate/world 只读集群元数据, 无需存盘参数)
type MongoConfig struct {
	URI    string `yaml:"uri"`
	DBName string `yaml:"db_name"`
}

// HomeRingConfig Home 节点一致性哈希环配置。
// 仅作为首次部署播种/Mongo 不可达时的启动兜底; 运行期真相源是 Mongo 环文档 (ringctl 改环 + 轮询热切)。
// Nodes 为当前环的物理节点列表；PrevNodes 为上一版环（在线扩容过渡期用于释放握手反查旧归属，平时为空）
type HomeRingConfig struct {
	Version   int      `yaml:"version"`
	Nodes     []string `yaml:"nodes"`
	PrevNodes []string `yaml:"prev_nodes"`
}

// GateConfig 网关节点专有配置
type GateConfig struct {
	Node      NodeConfig `yaml:"node"`
	WebSocket struct {
		ListenAddr string `yaml:"listen_addr"`
	} `yaml:"websocket"`
	Database  MongoConfig    `yaml:"database"` // 集群环文档读取 (可缺省, 缺省时退化为 yaml 静态环)
	HomeRing  HomeRingConfig `yaml:"home_ring"`
	WorldNode string         `yaml:"world_node"` // World 节点名, 用于寻址 world_actor
}

// HomeConfig 逻辑服节点专有配置
type HomeConfig struct {
	Node     NodeConfig `yaml:"node"`
	Database struct {
		URI         string `yaml:"uri"`
		DBName      string `yaml:"db_name"`
		SaveWorkers int    `yaml:"save_workers"`
		BatchSize   int    `yaml:"batch_size"`
	} `yaml:"database"`
	HomeRing  HomeRingConfig `yaml:"home_ring"`
	WorldNode string         `yaml:"world_node"` // World 节点名, 用于寻址 world_actor
}

// WorldConfig 地图服节点专有配置
type WorldConfig struct {
	Node     NodeConfig     `yaml:"node"`
	Database MongoConfig    `yaml:"database"`  // 集群环文档读取 (可缺省, 缺省时退化为 yaml 静态环)
	HomeRing HomeRingConfig `yaml:"home_ring"` // world → home 主动通知时的归属寻址
}

// LoadGateConfig 载入网关配置
func LoadGateConfig(path string) (*GateConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg GateConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadHomeConfig 载入逻辑服配置
func LoadHomeConfig(path string) (*HomeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg HomeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadWorldConfig 载入世界服配置
func LoadWorldConfig(path string) (*WorldConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg WorldConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
