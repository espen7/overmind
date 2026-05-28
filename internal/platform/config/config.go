package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"overmind/internal/platform/app"
)

func Load(configPath string) (app.Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configPath)
	v.AddConfigPath("./configs")
	v.AddConfigPath(".")
	v.SetEnvPrefix("OVERMIND")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return app.Config{}, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg app.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return app.Config{}, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return app.Config{}, err
	}
	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("services.gateway.name", "gateway")
	v.SetDefault("services.gateway.host", "127.0.0.1")
	v.SetDefault("services.gateway.port", 8080)
	v.SetDefault("services.portal.name", "portal")
	v.SetDefault("services.portal.host", "127.0.0.1")
	v.SetDefault("services.portal.port", 8081)
	v.SetDefault("services.world.name", "world")
	v.SetDefault("services.world.host", "127.0.0.1")
	v.SetDefault("services.world.port", 8082)
	v.SetDefault("log.level", "debug")
	v.SetDefault("log.encoding", "console")
	v.SetDefault("world.scene.width", 1000)
	v.SetDefault("world.scene.height", 1000)
	v.SetDefault("world.scene.cell_size", 100)
}
