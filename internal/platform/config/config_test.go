package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsPortalAndWorldServiceConfig(t *testing.T) {
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
  world:
    name: world
    host: 127.0.0.1
    port: 8082
world:
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
	if cfg.Services.World.Name != "world" {
		t.Fatalf("expected world service name, got %q", cfg.Services.World.Name)
	}
	if cfg.Services.World.Port != 8082 {
		t.Fatalf("expected world port 8082, got %d", cfg.Services.World.Port)
	}
}

func TestLoadReadsActorAndMongoConfig(t *testing.T) {
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
  player:
    name: player
    host: 127.0.0.1
    port: 8082
  world:
    name: world
    host: 127.0.0.1
    port: 8083
actor:
  system: overmind
  host: 127.0.0.1
  port: 9001
mongo:
  uri: mongodb://127.0.0.1:27017
  database: overmind
world:
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
	if cfg.Services.Player.Name != "player" {
		t.Fatalf("expected player service name, got %q", cfg.Services.Player.Name)
	}
	if cfg.Services.Player.Port != 8082 {
		t.Fatalf("expected player port 8082, got %d", cfg.Services.Player.Port)
	}
	if cfg.Actor.System != "overmind" {
		t.Fatalf("expected actor system overmind, got %q", cfg.Actor.System)
	}
	if cfg.Actor.Port != 9001 {
		t.Fatalf("expected actor port 9001, got %d", cfg.Actor.Port)
	}
	if cfg.Mongo.URI != "mongodb://127.0.0.1:27017" {
		t.Fatalf("expected mongo uri, got %q", cfg.Mongo.URI)
	}
	if cfg.Mongo.Database != "overmind" {
		t.Fatalf("expected mongo database overmind, got %q", cfg.Mongo.Database)
	}
}
