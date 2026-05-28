# Overmind Core Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `overmind` around `gateway`, `portal`, and `world` services with login, scene entry, AOI visibility, and basic combat, while explicitly excluding monitoring and ops features.

**Architecture:** Replace the current ProtoActor-centered skeleton with a lightweight service layout. Keep boundaries explicit by separating shared bootstrap utilities, protocol definitions, portal logic, world logic, and gateway transport so later RPC or cluster adapters can be added without rewriting domain code.

**Tech Stack:** Go 1.25, Gorilla WebSocket, Protobuf, Viper, Zap, in-memory repositories, table-driven Go tests

---

## File Map

### Create

- `cmd/portal/main.go`
- `cmd/world/main.go`
- `api/proto/portal/portal.proto`
- `api/proto/world/world.proto`
- `internal/platform/app/app.go`
- `internal/platform/config/config.go`
- `internal/platform/logging/logging.go`
- `internal/platform/clock/clock.go`
- `internal/portal/domain/account.go`
- `internal/portal/service/portal_service.go`
- `internal/portal/repository/memory_repository.go`
- `internal/portal/transport/handler.go`
- `internal/world/domain/entity.go`
- `internal/world/domain/scene.go`
- `internal/world/service/scene_service.go`
- `internal/world/service/combat_service.go`
- `internal/world/service/message_service.go`
- `internal/world/repository/memory_world.go`
- `internal/world/transport/handler.go`
- `pkg/aoi/grid.go`
- `pkg/aoi/grid_test.go`
- `internal/portal/service/portal_service_test.go`
- `internal/world/service/combat_service_test.go`
- `internal/world/service/scene_service_test.go`

### Modify

- `cmd/gateway/main.go`
- `configs/config.yaml`
- `go.mod`
- `scripts/proto_gen.bat`
- `scripts/proto_gen.sh`
- `internal/gateway/net/server.go`
- `internal/gateway/net/ws_server.go`
- `internal/gateway/protocol/packet.go`

### Remove or Replace

- `cmd/portal/main.go`
- `cmd/portal/wire.go`
- `cmd/portal/wire_gen.go`
- `cmd/world/main.go`
- `cmd/home/main.go`
- `cmd/admin/main.go`
- `internal/portal/actor.go`
- `internal/world/world/actor.go`
- `internal/world/world/cell.go`
- `internal/gateway/channel/actor.go`

## Task 1: Replace the old startup skeleton with the new service layout

**Files:**
- Create: `cmd/portal/main.go`, `cmd/world/main.go`, `internal/platform/app/app.go`, `internal/platform/config/config.go`, `internal/platform/logging/logging.go`, `internal/platform/clock/clock.go`
- Modify: `cmd/gateway/main.go`, `configs/config.yaml`
- Remove or Replace: `cmd/portal/main.go`, `cmd/world/main.go`, `cmd/home/main.go`, `cmd/admin/main.go`
- Test: `go test ./internal/platform/...`

- [ ] **Step 1: Write the failing bootstrap compile test**

```go
package app

import "testing"

func TestServiceConfigValidateRequiresNamesAndPorts(t *testing.T) {
	cfg := Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error for empty config")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/...`
Expected: FAIL with `undefined: Config` in `internal/platform/app`

- [ ] **Step 3: Write minimal platform bootstrap implementation**

```go
package app

import "fmt"

type ServiceConfig struct {
	Name string
	Port int
}

type Config struct {
	Gateway ServiceConfig
	Portal  ServiceConfig
	Game    ServiceConfig
}

func (c Config) Validate() error {
	if c.Gateway.Name == "" || c.Gateway.Port == 0 {
		return fmt.Errorf("gateway config is required")
	}
	if c.Portal.Name == "" || c.Portal.Port == 0 {
		return fmt.Errorf("portal config is required")
	}
	if c.Game.Name == "" || c.Game.Port == 0 {
		return fmt.Errorf("game config is required")
	}
	return nil
}
```

- [ ] **Step 4: Replace service entrypoints**

```go
package main

import (
	"log"
	"overmind/internal/platform/config"
	"overmind/internal/platform/logging"
)

func main() {
	cfg, err := config.Load(".")
	if err != nil {
		log.Fatal(err)
	}
	logging.Init(cfg.Log.Level)
	logging.L().Info("portal service starting")
}
```

- [ ] **Step 5: Update configuration file**

```yaml
services:
  gateway:
    name: "gateway"
    port: 8080
  portal:
    name: "portal"
    port: 8081
  game:
    name: "game"
    port: 8082

log:
  level: "debug"
  encoding: "console"

game:
  scene:
    width: 1000
    height: 1000
    cell_size: 100
```

- [ ] **Step 6: Run tests and compile checks**

Run: `go test ./internal/platform/... && go test ./cmd/...`
Expected: PASS for platform package tests and all commands compile

- [ ] **Step 7: Commit**

```bash
git add cmd/portal/main.go cmd/world/main.go cmd/gateway/main.go configs/config.yaml internal/platform
git commit -m "refactor: rebuild service bootstrap layout"
```

## Task 2: Define protobuf contracts and gateway packet framing

**Files:**
- Create: `api/proto/portal/portal.proto`, `api/proto/world/world.proto`
- Modify: `scripts/proto_gen.bat`, `scripts/proto_gen.sh`, `internal/gateway/protocol/packet.go`, `go.mod`
- Test: `go test ./internal/gateway/protocol/...`

- [ ] **Step 1: Write the failing packet codec test**

```go
package protocol

import "testing"

func TestPacketRoundTrip(t *testing.T) {
	packet := Packet{Type: 1001, Payload: []byte("hello")}
	encoded := Encode(packet)
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode returned error: %v", err)
	}
	if decoded.Type != packet.Type || string(decoded.Payload) != "hello" {
		t.Fatalf("unexpected decode result: %+v", decoded)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway/protocol/...`
Expected: FAIL with `undefined: Packet`, `Encode`, or `Decode`

- [ ] **Step 3: Define portal and world protobufs**

```proto
syntax = "proto3";

package portal;

option go_package = "overmind/pkg/pb/portal;portalpb";

message LoginRequest {
  string username = 1;
  string password = 2;
}

message LoginResponse {
  string token = 1;
  int64 player_id = 2;
  string player_name = 3;
  int64 scene_id = 4;
  int32 x = 5;
  int32 y = 6;
  int32 error_code = 7;
  string error_message = 8;
}
```

```proto
syntax = "proto3";

package world;

option go_package = "overmind/pkg/pb/world;worldpb";

message EnterSceneRequest {}

message MoveRequest {
  int32 x = 1;
  int32 y = 2;
}

message AttackRequest {
  int64 target_id = 1;
}
```

- [ ] **Step 4: Implement packet framing**

```go
package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type Packet struct {
	Type    uint16
	Payload []byte
}

func Encode(packet Packet) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 6+len(packet.Payload)))
	_ = binary.Write(buf, binary.BigEndian, packet.Type)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(packet.Payload)))
	buf.Write(packet.Payload)
	return buf.Bytes()
}

func Decode(data []byte) (Packet, error) {
	if len(data) < 6 {
		return Packet{}, fmt.Errorf("packet too short")
	}
	size := binary.BigEndian.Uint32(data[2:6])
	if len(data[6:]) != int(size) {
		return Packet{}, fmt.Errorf("packet size mismatch")
	}
	return Packet{
		Type:    binary.BigEndian.Uint16(data[:2]),
		Payload: append([]byte(nil), data[6:]...),
	}, nil
}
```

- [ ] **Step 5: Regenerate protobuf code**

Run: `./scripts/proto_gen.sh` on Unix or `.\scripts\proto_gen.bat` on Windows
Expected: `pkg/pb/portal` and `pkg/pb/world` generated without errors

- [ ] **Step 6: Run tests**

Run: `go test ./internal/gateway/protocol/...`
Expected: PASS with packet round-trip green

- [ ] **Step 7: Commit**

```bash
git add api/proto scripts/proto_gen.bat scripts/proto_gen.sh internal/gateway/protocol/packet.go pkg/pb
git commit -m "feat: define portal and world wire protocol"
```

## Task 3: Build portal service with in-memory accounts and session tokens

**Files:**
- Create: `internal/portal/domain/account.go`, `internal/portal/service/portal_service.go`, `internal/portal/repository/memory_repository.go`, `internal/portal/transport/handler.go`, `internal/portal/service/portal_service_test.go`
- Test: `internal/portal/service/portal_service_test.go`

- [ ] **Step 1: Write the failing portal service tests**

```go
package service

import "testing"

func TestLoginReturnsTokenAndPlayer(t *testing.T) {
	svc := New(NewMemoryRepository())
	resp, err := svc.Login("demo", "demo")
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	if resp.Token == "" || resp.PlayerID == 0 {
		t.Fatalf("expected token and player id, got %+v", resp)
	}
}

func TestValidateRejectsUnknownToken(t *testing.T) {
	svc := New(NewMemoryRepository())
	if _, err := svc.Validate("missing"); err == nil {
		t.Fatal("expected validate to fail for unknown token")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/portal/service -run TestLoginReturnsTokenAndPlayer -v`
Expected: FAIL with `undefined: New` or `NewMemoryRepository`

- [ ] **Step 3: Implement minimal portal domain and repository**

```go
package domain

type Account struct {
	Username string
	Password string
	PlayerID int64
	Name     string
	SceneID  int64
	X        int32
	Y        int32
}
```

```go
package repository

type MemoryRepository struct {
	accounts map[string]domain.Account
	tokens   map[string]domain.Account
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		accounts: map[string]domain.Account{
			"demo": {Username: "demo", Password: "demo", PlayerID: 10001, Name: "DemoKnight", SceneID: 1, X: 120, Y: 120},
		},
		tokens: make(map[string]domain.Account),
	}
}
```

- [ ] **Step 4: Implement portal service behavior**

```go
package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type LoginResult struct {
	Token      string
	PlayerID   int64
	PlayerName string
	SceneID    int64
	X          int32
	Y          int32
}

func (s *PortalService) Login(username, password string) (LoginResult, error) {
	account, err := s.repo.FindAccount(username)
	if err != nil || account.Password != password {
		return LoginResult{}, fmt.Errorf("invalid credentials")
	}
	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)
	s.repo.SaveToken(token, account)
	return LoginResult{
		Token: token, PlayerID: account.PlayerID, PlayerName: account.Name,
		SceneID: account.SceneID, X: account.X, Y: account.Y,
	}, nil
}
```

- [ ] **Step 5: Add transport-facing handler**

```go
package transport

import portalpb "overmind/pkg/pb/portal"

func (h Handler) Login(req *portalpb.LoginRequest) (*portalpb.LoginResponse, error) {
	result, err := h.service.Login(req.GetUsername(), req.GetPassword())
	if err != nil {
		return &portalpb.LoginResponse{ErrorCode: 401, ErrorMessage: err.Error()}, nil
	}
	return &portalpb.LoginResponse{
		Token: result.Token, PlayerId: result.PlayerID, PlayerName: result.PlayerName,
		SceneId: result.SceneID, X: result.X, Y: result.Y,
	}, nil
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/portal/...`
Expected: PASS with both login and token validation tests green

- [ ] **Step 7: Commit**

```bash
git add internal/portal
git commit -m "feat: add in-memory portal service"
```

## Task 4: Implement AOI scene management and basic combat with tests first

**Files:**
- Create: `pkg/aoi/grid.go`, `pkg/aoi/grid_test.go`, `internal/world/domain/entity.go`, `internal/world/domain/scene.go`, `internal/world/service/scene_service.go`, `internal/world/service/combat_service.go`, `internal/world/service/message_service.go`, `internal/world/repository/memory_world.go`, `internal/world/service/combat_service_test.go`, `internal/world/service/scene_service_test.go`
- Test: `pkg/aoi/grid_test.go`, `internal/world/service/scene_service_test.go`, `internal/world/service/combat_service_test.go`

- [ ] **Step 1: Write the failing AOI and combat tests**

```go
package aoi

import "testing"

func TestMoveReturnsPlayersFromCurrentAndNeighborCells(t *testing.T) {
	grid := NewGrid(1000, 1000, 100)
	grid.Upsert(1, 100, 100)
	grid.Upsert(2, 180, 180)
	visible := grid.VisibleTo(1)
	if len(visible) != 1 || visible[0] != 2 {
		t.Fatalf("expected player 2 to be visible, got %v", visible)
	}
}
```

```go
package service

import "testing"

func TestAttackReducesMonsterHPAndMarksDeath(t *testing.T) {
	world := NewMemoryWorld()
	svc := NewCombatService(world)
	world.SeedMonster(1, 9001, 40, 20, 120, 120)
	result, err := svc.Attack(10001, 1, 9001)
	if err != nil {
		t.Fatalf("attack returned error: %v", err)
	}
	if result.MonsterHP >= 40 {
		t.Fatalf("expected hp drop, got %d", result.MonsterHP)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/aoi ./internal/world/service -v`
Expected: FAIL with missing constructors and methods

- [ ] **Step 3: Implement AOI grid**

```go
package aoi

type Grid struct {
	cellSize int32
	entities map[int64]Point
}

type Point struct {
	X int32
	Y int32
}

func NewGrid(width, height, cellSize int32) *Grid {
	return &Grid{cellSize: cellSize, entities: make(map[int64]Point)}
}

func (g *Grid) Upsert(id int64, x, y int32) {
	g.entities[id] = Point{X: x, Y: y}
}
```

- [ ] **Step 4: Implement scene and combat services**

```go
package service

type AttackResult struct {
	SceneID   int64
	AttackerID int64
	TargetID  int64
	Damage    int32
	MonsterHP int32
	Dead      bool
}

func (s *CombatService) Attack(playerID, sceneID, targetID int64) (AttackResult, error) {
	monster, err := s.world.FindMonster(sceneID, targetID)
	if err != nil {
		return AttackResult{}, err
	}
	damage := int32(10)
	monster.HP -= damage
	dead := monster.HP <= 0
	if dead {
		monster.HP = 0
	}
	s.world.SaveMonster(sceneID, monster)
	return AttackResult{
		SceneID: sceneID, AttackerID: playerID, TargetID: targetID,
		Damage: damage, MonsterHP: monster.HP, Dead: dead,
	}, nil
}
```

- [ ] **Step 5: Add scene message generation**

```go
package service

import worldpb "overmind/pkg/pb/world"

func BuildMoveBroadcast(playerID int64, x, y int32) *worldpb.SceneEvent {
	return &worldpb.SceneEvent{
		Event: &worldpb.SceneEvent_Move{
			Move: &worldpb.MoveBroadcast{PlayerId: playerID, X: x, Y: y},
		},
	}
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./pkg/aoi ./internal/world/...`
Expected: PASS with AOI visibility, scene entry, and combat behavior covered

- [ ] **Step 7: Commit**

```bash
git add pkg/aoi internal/world
git commit -m "feat: add scene aoi and combat services"
```

## Task 5: Rebuild gateway flow and finish the end-to-end smoke path

**Files:**
- Modify: `cmd/gateway/main.go`, `internal/gateway/net/server.go`, `internal/gateway/net/ws_server.go`, `internal/gateway/protocol/packet.go`
- Create: `internal/portal/transport/handler.go`, `internal/world/transport/handler.go`
- Test: `go test ./...`

- [ ] **Step 1: Write the failing gateway session test**

```go
package net

import "testing"

func TestSessionBindStoresPlayerAndToken(t *testing.T) {
	session := NewSession("conn-1")
	session.Bind(10001, "token-1")
	if session.PlayerID() != 10001 {
		t.Fatalf("expected bound player id")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway/net -run TestSessionBindStoresPlayerAndToken -v`
Expected: FAIL with missing `Session` API

- [ ] **Step 3: Implement gateway session and routing**

```go
package net

type Session struct {
	connID   string
	playerID int64
	token    string
}

func NewSession(connID string) *Session { return &Session{connID: connID} }
func (s *Session) Bind(playerID int64, token string) {
	s.playerID = playerID
	s.token = token
}
func (s *Session) PlayerID() int64 { return s.playerID }
```

- [ ] **Step 4: Hook gateway requests to portal and world handlers**

```go
switch packet.Type {
case MessageTypeLogin:
	var req portalpb.LoginRequest
	if err := proto.Unmarshal(packet.Payload, &req); err != nil {
		return err
	}
	resp, err := g.portalHandler.Login(&req)
	if err != nil {
		return err
	}
	session.Bind(resp.GetPlayerId(), resp.GetToken())
	return g.writeProto(conn, MessageTypeLoginResp, resp)
case MessageTypeEnterScene:
	return g.handleEnterScene(conn, session, packet.Payload)
case MessageTypeMove:
	return g.handleMove(conn, session, packet.Payload)
case MessageTypeAttack:
	return g.handleAttack(conn, session, packet.Payload)
}
```

- [ ] **Step 5: Run full verification**

Run: `go test ./...`
Expected: PASS

Run: `go build ./cmd/gateway ./cmd/portal ./cmd/world`
Expected: PASS with three binaries built successfully

- [ ] **Step 6: Smoke test the local flow**

Run: `go run ./cmd/portal` in one terminal, `go run ./cmd/world` in a second, and `go run ./cmd/gateway` in a third
Expected: all three services start cleanly with no panic and log their listening ports

- [ ] **Step 7: Commit**

```bash
git add cmd/gateway internal/gateway internal/portal/transport internal/world/transport
git commit -m "feat: wire gateway portal and world flow"
```

## Self-Review Checklist

- Spec coverage:
  - service split: Tasks 1 and 5
  - protobuf and protocol: Task 2
  - portal login/session flow: Task 3
  - scene entry, AOI, combat: Task 4
  - verification: Task 5
- Placeholder scan:
  - No `TODO`, `TBD`, or "similar to task N" placeholders remain
- Type consistency:
  - `LoginResult`, `Packet`, `Session`, `AttackResult`, and `BuildMoveBroadcast` are defined before later usage
