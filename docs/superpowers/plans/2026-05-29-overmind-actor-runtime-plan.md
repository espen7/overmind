# Overmind Actor Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将当前 `gateway -> handler` 的进程内调用重构为参考 `antares-main` 的 `gateway + portal + player + world` actor 运行时骨架，并打通 `channel -> world -> player` 登录链路。

**Architecture:** 第一阶段先引入 `protoactor-go` 和新的 actor 消息模型，新增独立 `player` 节点与 `player/world/channel` actor 骨架，再把 `gateway` 的登录和协议分发改成 actor 路由。已有世界场景与战斗逻辑先保留在 `world` 侧服务里，通过 `WorldActor` 适配调用，MongoDB 持久化只先落配置和抽象边界，不在本轮直接完成整套落库。

**Tech Stack:** Go 1.25、`github.com/asynkron/protoactor-go`、Gorilla WebSocket、Viper、Zap、Protobuf

---

## File Structure

### New files

- `cmd/player/main.go`
  `player` 独立进程入口。
- `internal/cluster/runtime/runtime.go`
  `protoactor-go` actor system 初始化与共享启动入口。
- `internal/cluster/messages/messages.go`
  `channel/player/world` 之间的内部消息定义。
- `internal/gateway/actor/channel_actor.go`
  `channelActor`，承载连接生命周期和消息路由。
- `internal/player/actor/player_actor.go`
  `playerActor`，承载玩家登录绑定和玩家主状态入口。
- `internal/player/service/login_service.go`
  `player` 侧登录/创建逻辑。
- `internal/world/actor/world_actor.go`
  `WorldActor`，承载世界登录入口和世界侧协议执行。
- `internal/world/service/login_service.go`
  `world` 侧登录协调服务，负责 `playerAbstract` 查询与玩家创建决策。
- `internal/platform/config/mongo.go`
  MongoDB 配置结构体与校验。

### Modified files

- `go.mod`
  引入 `protoactor-go`。
- `configs/config.yaml`
  增加 `player`、actor 运行时、MongoDB 配置。
- `internal/platform/app/app.go`
  增加 `player` 服务配置与校验。
- `internal/platform/config/config.go`
  读取新增配置。
- `cmd/gateway/main.go`
  用 actor runtime 替换 handler 直连。
- `cmd/world/main.go`
  启动世界 actor 节点。
- `cmd/portal/main.go`
  保持轻量入口，但从热链路中解耦。
- `internal/gateway/net/ws_server.go`
  改成只处理 WebSocket 边界与 actor session 适配。
- `README.md`
  更新当前架构与运行说明。

### Test files

- `internal/platform/config/config_test.go`
- `internal/cluster/messages/messages_test.go`
- `internal/player/actor/player_actor_test.go`
- `internal/world/actor/world_actor_test.go`
- `internal/gateway/actor/channel_actor_test.go`

---

### Task 1: 引入 actor 依赖与运行时配置

**Files:**
- Modify: `go.mod`
- Modify: `configs/config.yaml`
- Modify: `internal/platform/app/app.go`
- Modify: `internal/platform/config/config.go`
- Modify: `internal/platform/config/config_test.go`
- Create: `internal/platform/config/mongo.go`
- Test: `internal/platform/config/config_test.go`

- [ ] **Step 1: 先写配置测试，定义 `player`、actor、MongoDB 配置行为**

```go
func TestLoadReadsActorAndMongoConfig(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`
services:
  gateway: { name: gateway, host: 127.0.0.1, port: 8080 }
  portal:  { name: portal, host: 127.0.0.1, port: 8081 }
  player:  { name: player, host: 127.0.0.1, port: 8082 }
  world:   { name: world, host: 127.0.0.1, port: 8083 }
actor:
  system: overmind
  host: 127.0.0.1
  port: 9001
mongo:
  uri: mongodb://127.0.0.1:27017
  database: overmind
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Services.Player.Name != "player" {
		t.Fatalf("expected player service, got %q", cfg.Services.Player.Name)
	}
	if cfg.Actor.System != "overmind" {
		t.Fatalf("expected actor system overmind, got %q", cfg.Actor.System)
	}
	if cfg.Mongo.Database != "overmind" {
		t.Fatalf("expected mongo database overmind, got %q", cfg.Mongo.Database)
	}
}
```

- [ ] **Step 2: 跑配置测试，确认当前必然失败**

Run: `go test ./internal/platform/config -run TestLoadReadsActorAndMongoConfig -v`

Expected: FAIL，报 `Services.Player` / `Actor` / `Mongo` 字段不存在或未加载。

- [ ] **Step 3: 最小实现配置结构和默认值**

```go
type ActorConfig struct {
	System string `mapstructure:"system"`
	Host   string `mapstructure:"host"`
	Port   int    `mapstructure:"port"`
}

type MongoConfig struct {
	URI      string `mapstructure:"uri"`
	Database string `mapstructure:"database"`
}

type Services struct {
	Gateway ServiceConfig `mapstructure:"gateway"`
	Portal  ServiceConfig `mapstructure:"portal"`
	Player  ServiceConfig `mapstructure:"player"`
	World   ServiceConfig `mapstructure:"world"`
}

type Config struct {
	Services Services           `mapstructure:"services"`
	Actor    ActorConfig        `mapstructure:"actor"`
	Mongo    MongoConfig        `mapstructure:"mongo"`
	World    WorldRuntimeConfig `mapstructure:"world"`
	Log      LogConfig          `mapstructure:"log"`
}
```

- [ ] **Step 4: 再跑配置测试，确认转绿**

Run: `go test ./internal/platform/config -run TestLoadReadsActorAndMongoConfig -v`

Expected: PASS

- [ ] **Step 5: 提交这一小步**

```bash
git add go.mod configs/config.yaml internal/platform/app/app.go internal/platform/config/config.go internal/platform/config/config_test.go internal/platform/config/mongo.go
git commit -m "feat: add actor and mongo runtime config"
```

### Task 2: 建立内部 actor 消息模型与运行时入口

**Files:**
- Create: `internal/cluster/messages/messages.go`
- Create: `internal/cluster/messages/messages_test.go`
- Create: `internal/cluster/runtime/runtime.go`
- Test: `internal/cluster/messages/messages_test.go`

- [ ] **Step 1: 先写消息测试，固定登录链路消息契约**

```go
func TestPlayerLoginReqTargetsPlayerIdentity(t *testing.T) {
	req := PlayerLoginReq{
		PlayerID: 1001,
		WorldID:  1,
		Account:  "demo",
	}

	if got := req.Identity(); got != "player/1001" {
		t.Fatalf("expected identity player/1001, got %q", got)
	}
}

func TestWorldLoginReqTargetsWorldIdentity(t *testing.T) {
	req := WorldLoginReq{WorldID: 7}
	if got := req.Identity(); got != "world/7" {
		t.Fatalf("expected identity world/7, got %q", got)
	}
}
```

- [ ] **Step 2: 跑消息测试，确认失败**

Run: `go test ./internal/cluster/messages -run 'Test(Player|World)LoginReq' -v`

Expected: FAIL，提示消息类型不存在。

- [ ] **Step 3: 写最小消息定义和运行时入口**

```go
type IdentityMessage interface {
	Identity() string
}

type WorldLoginReq struct {
	WorldID  int64
	Account  string
	ConnID   string
	ReplyPID *actor.PID
}

func (m WorldLoginReq) Identity() string {
	return fmt.Sprintf("world/%d", m.WorldID)
}

type PlayerLoginReq struct {
	PlayerID int64
	WorldID  int64
	Account  string
	ConnID   string
	ReplyPID *actor.PID
}

func (m PlayerLoginReq) Identity() string {
	return fmt.Sprintf("player/%d", m.PlayerID)
}
```

- [ ] **Step 4: 再跑消息测试**

Run: `go test ./internal/cluster/messages -run 'Test(Player|World)LoginReq' -v`

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/cluster/messages/messages.go internal/cluster/messages/messages_test.go internal/cluster/runtime/runtime.go
git commit -m "feat: add actor runtime messages"
```

### Task 3: 新增 `playerActor` 与登录绑定逻辑

**Files:**
- Create: `cmd/player/main.go`
- Create: `internal/player/actor/player_actor.go`
- Create: `internal/player/actor/player_actor_test.go`
- Create: `internal/player/service/login_service.go`
- Modify: `internal/portal/domain/account.go`
- Test: `internal/player/actor/player_actor_test.go`

- [ ] **Step 1: 先写 `playerActor` 测试，固定“新连接覆盖旧连接”行为**

```go
func TestPlayerActorRebindsChannelAndExpiresOldOne(t *testing.T) {
	gotExpired := make(chan string, 1)
	props := actor.PropsFromFunc(func(ctx actor.Context) {
		switch msg := ctx.Message().(type) {
		case ChannelExpired:
			gotExpired <- msg.ConnID
		}
	})

	system := actor.NewActorSystem()
	oldPID := system.Root.Spawn(props)
	newPID := system.Root.Spawn(props)

	player := newPlayerActor(fakeLoginService{})
	pid := system.Root.Spawn(actor.PropsFromProducer(func() actor.Actor { return player }))

	system.Root.Send(pid, PlayerLoginReq{PlayerID: 1001, ConnID: "old", ReplyPID: oldPID})
	system.Root.Send(pid, PlayerLoginReq{PlayerID: 1001, ConnID: "new", ReplyPID: newPID})

	select {
	case expired := <-gotExpired:
		if expired != "old" {
			t.Fatalf("expected old conn expired, got %q", expired)
		}
	case <-time.After(time.Second):
		t.Fatal("expected old connection to expire")
	}
}
```

- [ ] **Step 2: 跑 `playerActor` 测试，确认失败**

Run: `go test ./internal/player/actor -run TestPlayerActorRebindsChannelAndExpiresOldOne -v`

Expected: FAIL，提示 `playerActor` 或消息处理缺失。

- [ ] **Step 3: 写最小 `playerActor` 和登录服务**

```go
type PlayerActor struct {
	service  LoginService
	connID   string
	replyPID *actor.PID
}

func (p *PlayerActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case messages.PlayerLoginReq:
		if p.replyPID != nil && p.connID != "" && p.connID != msg.ConnID {
			ctx.Send(p.replyPID, messages.ChannelExpired{ConnID: p.connID})
		}
		p.connID = msg.ConnID
		p.replyPID = msg.ReplyPID
		ctx.Respond(messages.PlayerLoginResp{
			PlayerID: msg.PlayerID,
			WorldID:  msg.WorldID,
			ConnID:   msg.ConnID,
		})
	}
}
```

- [ ] **Step 4: 再跑 `playerActor` 测试**

Run: `go test ./internal/player/actor -run TestPlayerActorRebindsChannelAndExpiresOldOne -v`

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/player/main.go internal/player/actor/player_actor.go internal/player/actor/player_actor_test.go internal/player/service/login_service.go internal/portal/domain/account.go
git commit -m "feat: add player actor login binding"
```

### Task 4: 新增 `WorldActor`，实现 `world -> player` 登录协调

**Files:**
- Create: `internal/world/actor/world_actor.go`
- Create: `internal/world/actor/world_actor_test.go`
- Create: `internal/world/service/login_service.go`
- Modify: `internal/world/repository/memory_world.go`
- Test: `internal/world/actor/world_actor_test.go`

- [ ] **Step 1: 先写 `WorldActor` 测试，固定“首次登录创建玩家、再次登录复用玩家”**

```go
func TestWorldActorCreatesAndReusesPlayerIdentity(t *testing.T) {
	service := newFakeWorldLoginService()
	world := NewWorldActor(service)

	// 第一次登录应创建 playerId
	// 第二次相同 account 登录应复用第一次生成的 playerId
}
```

- [ ] **Step 2: 跑测试，确认失败**

Run: `go test ./internal/world/actor -run TestWorldActorCreatesAndReusesPlayerIdentity -v`

Expected: FAIL

- [ ] **Step 3: 写最小 `WorldActor`**

```go
func (w *WorldActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case messages.WorldLoginReq:
		playerID, created, err := w.loginService.ResolvePlayer(msg.WorldID, msg.Account)
		if err != nil {
			ctx.Respond(messages.WorldLoginRejected{Reason: err.Error()})
			return
		}
		ctx.RequestWithCustomSender(w.playerResolver(playerID), messages.PlayerLoginReq{
			PlayerID: playerID,
			WorldID:  msg.WorldID,
			Account:  msg.Account,
			ConnID:   msg.ConnID,
			ReplyPID: msg.ReplyPID,
		}, ctx.Self())
		_ = created
	}
}
```

- [ ] **Step 4: 再跑 `WorldActor` 测试**

Run: `go test ./internal/world/actor -run TestWorldActorCreatesAndReusesPlayerIdentity -v`

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/world/actor/world_actor.go internal/world/actor/world_actor_test.go internal/world/service/login_service.go internal/world/repository/memory_world.go
git commit -m "feat: add world actor login coordination"
```

### Task 5: 改造 `gateway` 为 `channelActor` 路由

**Files:**
- Create: `internal/gateway/actor/channel_actor.go`
- Create: `internal/gateway/actor/channel_actor_test.go`
- Modify: `internal/gateway/net/ws_server.go`
- Modify: `cmd/gateway/main.go`
- Modify: `internal/gateway/net/session.go`
- Test: `internal/gateway/actor/channel_actor_test.go`

- [ ] **Step 1: 先写 `channelActor` 测试，固定“登录先投 world，业务按路由表投 player/world”**

```go
func TestChannelActorRoutesLoginToWorld(t *testing.T) {
	// 构造 fake world PID，断言收到 WorldLoginReq
}

func TestChannelActorRoutesMoveToWorld(t *testing.T) {
	// 构造已授权 channel actor，发送 MoveRequest，断言 world 收到 WorldEnvelope
}
```

- [ ] **Step 2: 跑测试，确认失败**

Run: `go test ./internal/gateway/actor -run TestChannelActorRoutes -v`

Expected: FAIL

- [ ] **Step 3: 写最小 `channelActor` 与网关接线**

```go
type ChannelActor struct {
	connID    string
	worldPID  *actor.PID
	playerPID *actor.PID
	writer    Writer
}

func (c *ChannelActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case LoginFrame:
		ctx.RequestWithCustomSender(c.worldPID, messages.WorldLoginReq{
			WorldID:  msg.WorldID,
			Account:  msg.Account,
			ConnID:   c.connID,
			ReplyPID: ctx.Self(),
		}, ctx.Self())
	case AuthorizedEnvelope:
		c.playerPID = msg.PlayerPID
	case ClientWorldEnvelope:
		ctx.Send(c.worldPID, msg)
	case ClientPlayerEnvelope:
		ctx.Send(c.playerPID, msg)
	}
}
```

- [ ] **Step 4: 再跑 `channelActor` 测试**

Run: `go test ./internal/gateway/actor -run TestChannelActorRoutes -v`

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/gateway/actor/channel_actor.go internal/gateway/actor/channel_actor_test.go internal/gateway/net/ws_server.go internal/gateway/net/session.go cmd/gateway/main.go
git commit -m "feat: route gateway traffic through channel actor"
```

### Task 6: 更新入口、验证与文档

**Files:**
- Modify: `cmd/world/main.go`
- Modify: `cmd/portal/main.go`
- Modify: `README.md`
- Modify: `configs/config.yaml`
- Test: `go test ./...`

- [ ] **Step 1: 更新入口与 README**

```md
- `gateway`：WebSocket + channelActor
- `player`：独立玩家 actor 节点
- `world`：独立世界 actor 节点
- `portal`：轻量账号入口预留，不在第一版热链路
```

- [ ] **Step 2: 跑全量测试**

Run: `go test ./...`

Expected: PASS

- [ ] **Step 3: 跑构建验证**

Run: `go build ./cmd/gateway ./cmd/portal ./cmd/player ./cmd/world`

Expected: PASS

- [ ] **Step 4: 跑 smoke 验证**

Run:

```powershell
go run ./cmd/player
go run ./cmd/world
go run ./cmd/gateway
```

Expected:

- `player` 启动成功
- `world` 启动成功
- `gateway` 启动成功
- `/healthz` 返回 200

- [ ] **Step 5: 提交**

```bash
git add cmd/world/main.go cmd/portal/main.go README.md configs/config.yaml
git commit -m "docs: describe actor runtime architecture"
```

## Self-Review

### Spec coverage

- `gateway + portal + player + world` 四节点：Task 1、Task 3、Task 4、Task 6
- `channel -> world -> player` 登录链路：Task 3、Task 4、Task 5
- `player` 为主数据真源：Task 3、Task 4
- `1 worldId = 1 WorldActor`：Task 4
- actor 热链路替代 handler 直调：Task 5
- MongoDB 作为后续底座配置：Task 1

### Placeholder scan

已避免 `TODO/TBD/implement later` 之类占位语。

### Type consistency

- 统一使用 `WorldLoginReq`、`PlayerLoginReq`、`ChannelExpired`
- 统一使用 `ConnID` 标识连接
- 统一由 `channelActor` 持有写回边界

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-29-overmind-actor-runtime-plan.md`.

默认执行路径：

1. Subagent-Driven（推荐）- 如果运行环境具备稳定的子代理执行能力，优先按任务逐个下发
2. Inline Execution - 当前会话直接按任务顺序实现

由于用户已明确要求“后面不用找我”，当前默认采用 **Inline Execution** 继续推进。
