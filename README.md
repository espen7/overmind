# Overmind

`Overmind` 是一个用 Go 编写的 SLG 游戏服务器项目。当前仓库正参考 `antares-main` 的 actor 架构，逐步把早期的进程内直调实现迁移为 `gateway + portal + player + world` 四节点模型。

> 当前分支处于重写与架构迁移阶段，重点是先把在线模型、玩家实体边界和 actor 运行时打稳，再逐步补齐跨节点通信和持久化能力。

## 项目状态

- 当前阶段：`pre-alpha / active rewrite`
- 当前目标：对齐 `antares-main` 的 actor 设计，建立可扩展的玩家实体与世界实体模型
- 当前范围：不包含监控运维、后台管理、跨服、家园系统和完整的 MongoDB 落地实现

如果你是第一次打开这个仓库，建议先把它理解成一个“正在从原型代码迁移到正式 actor 架构”的项目，而不是一个已经功能完备的成品服务器。

## 核心特性

- 基于 Go 实现，当前统一使用 `zap` 做结构化日志
- 采用 `gateway + portal + player + world` 四服务边界
- 已引入 `protoactor-go`，并开始向 `kind + identity` 的实体路由模型迁移
- `player` 与 `world` 明确分边界：
  - `player` 持有玩家私有主数据
  - `world` 持有世界运行所需投影与公共状态
- 已具备最小可运行的在线链路骨架：
  - 登录校验与令牌签发
  - `channelActor -> WorldActor -> PlayerActor` 登录协调
  - `PlayerActor` 初始化、在线绑定、空闲钝化、flush 骨架
  - 场景进入、AOI 可见性、基础怪物和基础战斗

## 架构概览

### 服务边界

- `gateway`
  - 负责 WebSocket 接入、连接生命周期和 `channelActor`
  - 当前已经不再直接装配 `world` 的 repository/service/handler
  - 登录成功后会通过 `player` shard proxy 把连接绑定到 `player` 实体
  - 进图、移动、战斗等世界消息改为通过 `world` shard proxy 转发到 `world` 实体
  - `gateway <-> player` 当前统一走 `Envelope + protobuf`，用于会话绑定、解绑和顶号踢旧连接
  - `gateway <-> world` 当前统一走 `Envelope + protobuf`，不再混用 JSON 返回体
- `portal`
  - 负责轻量账号入口与令牌能力
  - 第一版不进入核心 actor 热链路
- `player`
  - 负责 `PlayerActor`
  - 是玩家私有主数据与玩家级业务状态的入口
- `world`
  - 负责 `WorldActor`
  - 承担世界登录协调、场景状态、AOI 与基础战斗

### Actor 设计方向

当前 actor 运行时正从“直接拿本地 PID”过渡到“按 `kind + identity` 找唯一实体”的模式：

- `PlayerActor` 按 `playerID` 建模
- `WorldActor` 按 `worldID` 建模
- 当前仓库已经接入 `automanaged + disthash` 的 cluster provider 方案
- `gateway` 作为 cluster client，并通过 `player/world` shard proxy 访问实体
- `player` 与 `world` 作为 cluster member
- `world -> player` 登录协同已经切到 `Envelope + protobuf`
- `player` 进程已经统一装配 `player -> world` / `player -> player` 的 Envelope 远程出口
- 后续会继续补齐更多 `player/world` 业务协同和 `channelActor` 热链路

## 当前已完成

- `protoactor-go` 基础接入
- `player` 服务配置、actor runtime 配置、MongoDB 配置占位
- `channelActor / PlayerActor / WorldActor` 基础骨架
- `PlayerActor` 首版 `PlayerMem + PlayerActionMem` MongoDB traceable 持久化骨架
- `PlayerActor` 生命周期骨架：
  - 初始化
  - 在线绑定
  - 空闲钝化
  - flush
- 单节点内“一玩家一 actor”语义
- `kind + identity` 路由过渡层
- `gateway -> world` 跨进程 actor 通信
- `gateway -> player` 跨进程 actor 通信
- `world -> player` 登录协同跨进程 actor 通信
- `player -> world` / `player -> player` 统一远程出口骨架
- 本地登录账号校验
- 令牌签发与会话绑定
- 玩家进入场景
- AOI 九宫格可见性计算
- 基础怪物生成
- 基础攻击与伤害结算
- 世界事件广播

## 当前未完成

- 更完整的 `player -> world` / `player -> player` 业务消息
- `world` 与 `portal` 的 MongoDB 持久化
- 角色选择与多角色管理
- 技能、Buff、掉落、背包、任务
- 跨服、聊天、公会、邮件
- Prometheus / Grafana / Tracing 等运维组件

## 快速开始

### 环境要求

- Go `1.25.x`

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

默认配置位于 [configs/config.yaml](/D:/workspace/githut_repo/overmind/configs/config.yaml:1)，当前主要包含以下几类配置：

- `services`
  - 四个服务的名称、监听地址和端口
- `actor`
  - 按 `gateway / player / world` 拆分的 actor system 名称与节点地址
- `cluster`
  - cluster 名称、provider 类型和 automanaged 发现地址
- `mongo`
  - MongoDB 连接地址与数据库名
- `world.scene`
  - 场景尺寸与 AOI 网格参数
- `log`
  - 日志级别与输出格式（`console` / `json`）

## 目录结构

```text
api/proto/     # 协议定义
cmd/           # 各服务启动入口
configs/       # 配置文件
docs/          # 设计文档与实现计划
internal/      # 核心实现
pkg/           # 通用能力与 protobuf 生成代码
scripts/       # 辅助脚本
tools/         # 工具代码
```

更关键的子目录包括：

- `internal/cluster`
  - actor runtime、内部消息、identity 路由过渡层
- `internal/gateway`
  - WebSocket 传输层与 `channelActor`
- `internal/player`
  - `PlayerActor`、玩家运行时、玩家服务
- `internal/portal`
  - 门户领域、仓储、服务
- `internal/world`
  - `WorldActor`、世界服务、世界仓储
- `internal/platform`
  - 配置、日志、启动辅助

## 推荐阅读

如果你想快速看懂当前 actor 迁移进度，建议从下面这些文件开始：

- [cmd/gateway/main.go](/D:/workspace/githut_repo/overmind/cmd/gateway/main.go:1)
- [internal/gateway/app/app.go](/D:/workspace/githut_repo/overmind/internal/gateway/app/app.go:1)
- [cmd/player/main.go](/D:/workspace/githut_repo/overmind/cmd/player/main.go:1)
- [internal/world/app/app.go](/D:/workspace/githut_repo/overmind/internal/world/app/app.go:1)
- [internal/cluster/messages/messages.go](/D:/workspace/githut_repo/overmind/internal/cluster/messages/messages.go:1)
- [internal/cluster/messages/world_dispatch.go](/D:/workspace/githut_repo/overmind/internal/cluster/messages/world_dispatch.go:1)
- [internal/cluster/messages/player_session.go](/D:/workspace/githut_repo/overmind/internal/cluster/messages/player_session.go:1)
- [internal/cluster/runtime/router.go](/D:/workspace/githut_repo/overmind/internal/cluster/runtime/router.go:1)
- [internal/cluster/runtime/cluster_runtime.go](/D:/workspace/githut_repo/overmind/internal/cluster/runtime/cluster_runtime.go:1)
- [internal/cluster/runtime/local_virtual_cluster.go](/D:/workspace/githut_repo/overmind/internal/cluster/runtime/local_virtual_cluster.go:1)
- [internal/cluster/runtime/remote.go](/D:/workspace/githut_repo/overmind/internal/cluster/runtime/remote.go:1)
- [internal/gateway/actor/channel_actor.go](/D:/workspace/githut_repo/overmind/internal/gateway/actor/channel_actor.go:1)
- [internal/gateway/playerproxy/remote_client.go](/D:/workspace/githut_repo/overmind/internal/gateway/playerproxy/remote_client.go:1)
- [internal/gateway/worldproxy/remote_client.go](/D:/workspace/githut_repo/overmind/internal/gateway/worldproxy/remote_client.go:1)
- [internal/player/actor/player_actor.go](/D:/workspace/githut_repo/overmind/internal/player/actor/player_actor.go:1)
- [internal/world/actor/world_actor.go](/D:/workspace/githut_repo/overmind/internal/world/actor/world_actor.go:1)
- [internal/world/service/scene_service.go](/D:/workspace/githut_repo/overmind/internal/world/service/scene_service.go:1)

## 路线图

接下来最直接的演进方向是：

1. 把 `ws_server` 的登录和业务分发真正切到 `channelActor`
2. 在 `Envelope + protobuf` 之上继续补 `player -> world` / `player -> player` 的业务消息
3. 把 `ws_server` 真正收口到 `channelActor`，让 gateway 更接近 `antares-main`
4. 把 MongoDB 的数据加载、脏追踪和 flush 模型接进来
5. 让 `portal` 彻底退出第一版热链路，只保留外围入口能力

## 许可证

本项目采用 [Apache License 2.0](/D:/workspace/githut_repo/overmind/LICENSE:1)。
