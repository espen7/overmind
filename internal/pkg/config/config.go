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

// RouteConfig 描述集群静态路由
type RouteConfig struct {
	NodeName string `yaml:"node_name"`
	Host     string `yaml:"host"`
	Port     uint16 `yaml:"port"`
}

// GateConfig 网关节点专有配置
type GateConfig struct {
	Node      NodeConfig `yaml:"node"`
	WebSocket struct {
		ListenAddr string `yaml:"listen_addr"`
	} `yaml:"websocket"`
	Routes []RouteConfig `yaml:"routes"`
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
	Routes []RouteConfig `yaml:"routes"`
}

// WorldConfig 地图服节点专有配置
type WorldConfig struct {
	Node   NodeConfig    `yaml:"node"`
	Routes []RouteConfig `yaml:"routes"`
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
