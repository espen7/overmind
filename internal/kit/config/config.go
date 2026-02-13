package config

// Config 是根配置结构体，包含所有子模块的配置
type Config struct {
	Server ServerConfig `mapstructure:"server"` // 服务器基础配置
	Redis  RedisConfig  `mapstructure:"redis"`  // Redis 连接配置
	Log    LogConfig    `mapstructure:"log"`    // 日志系统配置
}

// ServerConfig 定义服务器通用属性
type ServerConfig struct {
	Name string `mapstructure:"name"` // 服务名称 (用于日志/注册中心)
	Port int    `mapstructure:"port"` // 监听端口
	Env  string `mapstructure:"env"`  // 运行环境: dev (开发), test (测试), prod (生产)
}

// RedisConfig 定义 Redis 连接属性
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`     // 地址 "host:port"
	Password string `mapstructure:"password"` // 密码 (可选)
	DB       int    `mapstructure:"db"`       // 数据库索引 (0-15)
}

// LogConfig 定义日志系统属性
type LogConfig struct {
	Level    string `mapstructure:"level"`    // 日志级别: debug, info, warn, error
	Encoding string `mapstructure:"encoding"` // 编码格式: console (彩色), json (结构化)
}
