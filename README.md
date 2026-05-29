# Overmind

`Overmind` 是一个使用 Go 编写的 SLG 游戏服务器项目，采用 `gateway + portal + player + world` 的服务边界，并基于 actor 模型组织在线连接、玩家实体与世界实体。

## 项目状态

项目目前处于早期开发阶段，核心目标是先稳定服务边界、消息模型与实体运行时，再逐步扩展完整的游戏玩法与持久化能力。

当前仓库不包含以下模块：

- 监控运维
- GM / Admin 后台
- 跨服能力
- 家园系统

## 核心特性

- 使用 Go 实现，日志统一基于 `zap`
- 服务边界清晰：`gateway`、`portal`、`player`、`world`
- actor 实体按 `kind + identity` 路由
- 支持 `ChannelActor`、`PlayerActor`、`WorldActor` 三类核心 actor
- 跨进程通信统一走 protobuf `Envelope`
- 客户端接入采用 `ClientPacket`
- 已具备基础登录、会话绑定、进图、AOI、基础战斗链路
- `PlayerActor` 已具备初始化、在线绑定、空闲钝化、Flush 骨架
- `player` 侧已接入首版 MongoDB 持久化骨架

## 架构概览

### 服务边界

- `gateway`
  - 负责 WebSocket 接入、连接生命周期与消息编解码
  - 通过 `player` shard proxy 和 `world` shard proxy 访问实体
- `portal`
  - 负责账号入口、认证与令牌能力
- `player`
  - 负责 `PlayerActor`
  - 持有玩家私有主数据与玩家级业务状态
- `world`
  - 负责 `WorldActor`
  - 持有世界运行所需的投影与公共状态

### Actor 模型

- `ChannelActor`
  - 表示一条网关连接对应的会话上下文
- `PlayerActor`
  - 表示单个玩家实体
- `WorldActor`
  - 表示单个世界实体

### 集群与路由

- `gateway` 作为 cluster client
- `player` 和 `world` 作为 cluster member
- 远程实体通过 `kind + identity` 定位
- shard proxy 负责把请求转发到目标实体

## 消息模型

项目中的消息分为三层：

### 1. ClientPacket

`ClientPacket` 用于 `client <-> gateway`。

职责：

- 表达客户端协议号 `msg_type`
- 表达二进制 payload
- 处理长度边界与编解码

位置：

- [internal/gateway/protocol/packet.go](/D:/workspace/githut_repo/overmind/internal/gateway/protocol/packet.go:1)

### 2. Envelope

`Envelope` 用于 `process <-> process`。

覆盖：

- `gateway -> player`
- `gateway -> world`
- `world -> player`
- `player -> world`
- `player -> player`

职责：

- 承载 trace / timestamp / sender / target 等服务间元数据
- 承载客户端语义载荷 `EdgeLetter`
- 承载内部系统载荷 `MeshLetter`

位置：

- [api/proto/kit/envelope.proto](/D:/workspace/githut_repo/overmind/api/proto/kit/envelope.proto:1)

### 3. LocalCommand / LocalEvent

`LocalCommand / LocalEvent` 仅用于 actor 进程内邮箱消息。

当前示例：

- `LoginCommand`
- `RouteToPlayerCommand`
- `RouteToWorldCommand`

位置：

- [internal/gateway/actor/channel_actor.go](/D:/workspace/githut_repo/overmind/internal/gateway/actor/channel_actor.go:1)

更完整的设计说明见：

- [docs/superpowers/specs/2026-05-29-overmind-message-layering-design.md](/D:/workspace/githut_repo/overmind/docs/superpowers/specs/2026-05-29-overmind-message-layering-design.md:1)

## 已实现能力

- `protoactor-go` 基础接入
- `gateway -> player` 跨进程 actor 通信
- `gateway -> world` 跨进程 actor 通信
- `world -> player` 登录协同跨进程 actor 通信
- `player -> world` / `player -> player` 统一远程出口骨架
- 本地账号登录校验
- 玩家会话绑定与解绑
- 顶号踢旧连接
- 玩家进入场景
- AOI 九宫格可见性计算
- 基础怪物生成
- 基础攻击与伤害结算
- 世界广播
- `PlayerMem + PlayerActionMem` 持久化骨架

## 快速开始

### 环境要求

- Go `1.25.x`
- MongoDB（启动 `player` 服务时需要可连接的 MongoDB）

### 运行测试

```powershell
go test ./...
```

### 构建服务

```powershell
go build ./cmd/gateway ./cmd/portal ./cmd/player ./cmd/world
```

### 启动服务

```powershell
go run ./cmd/player
go run ./cmd/portal
go run ./cmd/world
go run ./cmd/gateway
```

### 健康检查

- `http://127.0.0.1:8080/healthz`：gateway
- `http://127.0.0.1:8081/healthz`：portal
- `http://127.0.0.1:8082/healthz`：player
- `http://127.0.0.1:8083/healthz`：world

## 配置说明

默认配置位于 [configs/config.yaml](/D:/workspace/githut_repo/overmind/configs/config.yaml:1)。

当前主要配置包括：

- `services`
  - 四个服务的监听地址与端口
- `actors`
  - `gateway / player / world` 的 actor 节点地址
- `cluster`
  - cluster 名称、provider 与 discovery 地址
- `mongo`
  - MongoDB 连接地址与数据库名
- `world.scene`
  - 场景尺寸与 AOI 网格参数
- `log`
  - 日志级别与输出格式

## 目录结构

```text
api/proto/     # 协议定义
cmd/           # 服务启动入口
configs/       # 配置文件
docs/          # 设计文档
internal/      # 核心实现
pkg/           # 通用能力与 protobuf 生成代码
scripts/       # 辅助脚本
tools/         # 工具代码
```

关键目录：

- `internal/cluster`
  - 集群运行时、实体路由、内部消息
- `internal/gateway`
  - WebSocket 接入、协议编解码、ChannelActor
- `internal/player`
  - PlayerActor、玩家数据与玩家服务
- `internal/portal`
  - 认证入口与账号服务
- `internal/world`
  - WorldActor、场景服务与世界逻辑
- `internal/platform`
  - 配置、日志、启动与基础设施

## 推荐阅读

- [internal/gateway/app/app.go](/D:/workspace/githut_repo/overmind/internal/gateway/app/app.go:1)
- [internal/gateway/actor/channel_actor.go](/D:/workspace/githut_repo/overmind/internal/gateway/actor/channel_actor.go:1)
- [internal/gateway/protocol/packet.go](/D:/workspace/githut_repo/overmind/internal/gateway/protocol/packet.go:1)
- [internal/player/app/app.go](/D:/workspace/githut_repo/overmind/internal/player/app/app.go:1)
- [internal/player/actor/player_actor.go](/D:/workspace/githut_repo/overmind/internal/player/actor/player_actor.go:1)
- [internal/world/app/app.go](/D:/workspace/githut_repo/overmind/internal/world/app/app.go:1)
- [internal/world/actor/world_actor.go](/D:/workspace/githut_repo/overmind/internal/world/actor/world_actor.go:1)
- [internal/cluster/runtime/router.go](/D:/workspace/githut_repo/overmind/internal/cluster/runtime/router.go:1)
- [api/proto/kit/envelope.proto](/D:/workspace/githut_repo/overmind/api/proto/kit/envelope.proto:1)

## 路线图

- 将 `ws_server` 的登录与业务分发切入 `ChannelActor`
- 补齐 `player -> world` / `player -> player` 的业务消息
- 完善 `world` 与 `portal` 的持久化能力
- 扩展建筑、科技、部队、任务、邮件等 SLG 核心系统
- 完善负载拆分、区域划分与世界侧调度能力

## License

本项目采用 [Apache License 2.0](/D:/workspace/githut_repo/overmind/LICENSE:1)。
