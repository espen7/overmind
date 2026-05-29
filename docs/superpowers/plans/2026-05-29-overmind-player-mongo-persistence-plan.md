# Overmind Player Mongo Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `PlayerActor` 接入第一版 MongoDB 数据加载与刷盘能力，让玩家 actor 真正具备 `Init -> Tick -> Flush` 的持久化生命周期。

**Architecture:** 保持当前 `player` 为玩家主数据真源、`world` 为世界投影的边界不变，只实现 `player` 侧的首版聚合落库。运行时由 `PlayerActor` 持有 `DataManager`，激活时从 Mongo 加载或初始化玩家聚合，登录成功后更新登录快照，tick 和钝化时按脏标记 flush。

**Tech Stack:** Go 1.25、`protoactor-go`、MongoDB Go Driver v2、Viper、Zap

---

## File Structure

### New files

- `internal/platform/mongo/client.go`
  MongoDB client 连接与 database 打开入口。
- `internal/player/domain/player.go`
  玩家主聚合文档模型。
- `internal/player/repository/mongo_repository.go`
  玩家聚合的 Mongo 读写仓储。
- `internal/player/actor/mongo_data_manager.go`
  `PlayerActor` 的 Mongo 持久化 DataManager 实现。

### Modified files

- `go.mod`
  引入 MongoDB Go Driver v2。
- `cmd/player/main.go`
  初始化 Mongo client，并给 player actor 注册真实的 `ManagerFactory`。
- `internal/player/actor/data_manager.go`
  扩展 DataManager，使其能在登录成功后更新聚合内存态。
- `internal/player/actor/player_actor.go`
  在登录成功后通知 DataManager 更新登录快照。
- `README.md`
  补充当前 `player` 持久化状态说明。

---

### Task 1: 搭起 Mongo 持久化基础设施

**Files:**
- Modify: `go.mod`
- Create: `internal/platform/mongo/client.go`
- Create: `internal/player/domain/player.go`
- Create: `internal/player/repository/mongo_repository.go`

- [ ] 引入 MongoDB Go Driver v2。
- [ ] 新增 Mongo client 打开与关闭入口。
- [ ] 定义玩家聚合文档，首版只包含基础身份、世界归属、登录时间、版本号与更新时间。
- [ ] 定义 Mongo repository，提供按 `playerID` 加载、初始化和保存玩家聚合的能力。

### Task 2: 接通 PlayerActor 数据生命周期

**Files:**
- Modify: `internal/player/actor/data_manager.go`
- Create: `internal/player/actor/mongo_data_manager.go`
- Modify: `internal/player/actor/player_actor.go`

- [ ] 扩展 `DataManager` 接口，让登录成功后可以把登录快照写回内存态。
- [ ] 实现 `mongoDataManager`：
  - `Init` 时从 Mongo 加载或初始化玩家聚合
  - `Tick` 时仅在存在脏数据时尝试 flush
  - `Flush` 时把当前聚合整体 upsert 回 Mongo
- [ ] 在 `PlayerActor` 登录成功后调用 DataManager，同步最新 `worldID/account/lastLoginAt`。

### Task 3: 接入 player 进程启动入口

**Files:**
- Modify: `cmd/player/main.go`

- [ ] 启动时创建 Mongo client 和 `player` collection repository。
- [ ] 构造 Mongo-backed `ManagerFactory` 并注入 player kind。
- [ ] 保持本地启动失败时直接报错退出，不静默降级成 no-op。

### Task 4: 文档与验证

**Files:**
- Modify: `README.md`

- [ ] 在 README 中说明当前 `player` 已具备 Mongo 持久化骨架，而 `world/portal` 仍是内存实现。
- [ ] 运行 `go test ./...`。
- [ ] 运行 `go build ./cmd/gateway ./cmd/portal ./cmd/player ./cmd/world`。
