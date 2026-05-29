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
actors:
  gateway:
    system: overmind-gateway
    host: 127.0.0.1
    port: 9001
    discovery_port: 0
  player:
    system: overmind-player
    host: 127.0.0.1
    port: 9002
    discovery_port: 6332
  world:
    system: overmind-world
    host: 127.0.0.1
    port: 9003
    discovery_port: 6333
cluster:
  name: overmind-cluster
  provider: automanaged
  hosts:
    - 127.0.0.1:6332
    - 127.0.0.1:6333
  refresh_ttl_ms: 2000
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
	if cfg.Actors.Gateway.System != "overmind-gateway" {
		t.Fatalf("expected gateway actor system overmind-gateway, got %q", cfg.Actors.Gateway.System)
	}
	if cfg.Actors.Player.Port != 9002 {
		t.Fatalf("expected player actor port 9002, got %d", cfg.Actors.Player.Port)
	}
	if cfg.Actors.Player.DiscoveryPort != 6332 {
		t.Fatalf("expected player discovery port 6332, got %d", cfg.Actors.Player.DiscoveryPort)
	}
	if cfg.Actors.World.Port != 9003 {
		t.Fatalf("expected world actor port 9003, got %d", cfg.Actors.World.Port)
	}
	if cfg.Cluster.Name != "overmind-cluster" {
		t.Fatalf("expected cluster name overmind-cluster, got %q", cfg.Cluster.Name)
	}
	if cfg.Cluster.Provider != "automanaged" {
		t.Fatalf("expected cluster provider automanaged, got %q", cfg.Cluster.Provider)
	}
	if len(cfg.Cluster.Hosts) != 2 {
		t.Fatalf("expected 2 cluster hosts, got %d", len(cfg.Cluster.Hosts))
	}
	if cfg.Cluster.RefreshTTLMS != 2000 {
		t.Fatalf("expected cluster refresh ttl 2000, got %d", cfg.Cluster.RefreshTTLMS)
	}
	if cfg.Mongo.URI != "mongodb://127.0.0.1:27017" {
		t.Fatalf("expected mongo uri, got %q", cfg.Mongo.URI)
	}
	if cfg.Mongo.Database != "overmind" {
		t.Fatalf("expected mongo database overmind, got %q", cfg.Mongo.Database)
	}
}
