package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsPortalServiceConfig(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`
services:
  gateway:
    name: gateway
    host: 127.0.0.1
    port: 8080
  portal:
    name: portal
    host: 127.0.0.1
    port: 8081
  game:
    name: game
    host: 127.0.0.1
    port: 8082
game:
  scene:
    width: 1000
    height: 1000
    cell_size: 100
log:
  level: debug
  encoding: console
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Services.Portal.Name != "portal" {
		t.Fatalf("expected portal service name, got %q", cfg.Services.Portal.Name)
	}
	if cfg.Services.Portal.Port != 8081 {
		t.Fatalf("expected portal port 8081, got %d", cfg.Services.Portal.Port)
	}
}
