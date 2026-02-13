package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Load 从文件和环境变量中读取配置
// configPath: 配置文件所在目录
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// 1. 设置默认值 (防止配置文件缺失时报错)
	setDefaults(v)

	// 2. 配置 Viper 查找路径
	v.SetConfigName("config")    // 配置文件名 (不带后缀)
	v.SetConfigType("yaml")      // 强制使用 YAML 格式 (如果文件名没有后缀)
	v.AddConfigPath(configPath)  // 第一优先级: 指定目录
	v.AddConfigPath("./configs") // 第二优先级: 工作目录下的 configs
	v.AddConfigPath(".")         // 第三优先级: 当前目录

	// 3. 配置环境变量覆盖
	// 允许通过环境变量覆盖配置，例如: server.port -> OVERMIND_SERVER_PORT
	v.SetEnvPrefix("OVERMIND")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 4. 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// 配置文件未找到：仅使用默认值和环境变量，不报错
			fmt.Println("Config file not found, using defaults/env")
		} else {
			// 配置文件存在但解析错误：报错
			return nil, fmt.Errorf("fatal error config file: %w", err)
		}
	}

	// 5. 反序列化到结构体
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	return &c, nil
}

// setDefaults 设置配置项的默认值
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.name", "overmind-server")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.env", "dev")

	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)

	v.SetDefault("log.level", "debug")
	v.SetDefault("log.encoding", "console")
}
