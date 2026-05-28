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
	v.SetDefault("services.game.name", "game")
	v.SetDefault("services.game.host", "127.0.0.1")
	v.SetDefault("services.game.port", 8082)
	v.SetDefault("log.level", "debug")
	v.SetDefault("log.encoding", "console")
	v.SetDefault("game.scene.width", 1000)
	v.SetDefault("game.scene.height", 1000)
	v.SetDefault("game.scene.cell_size", 100)
}
