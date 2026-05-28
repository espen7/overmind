# Overmind

`overmind` 是一个用 Go 编写的 SLG 游戏服务器。当前分支正在参考 `antares-main` 的 actor 设计，把原来的 `gateway -> handler` 进程内直调链路，逐步迁移为 `gateway + portal + player + world` 四节点结构。

## 当前目标

当前这轮重构聚焦四个服务边界：

- `gateway`：负责 WebSocket 接入、连接生命周期和 `channelActor`
- `portal`：负责轻量账号入口预留，不放在第一版 actor 热链路
- `player`：负责 `playerActor`、在线绑定和玩家主数据入口
- `world`：负责 `WorldActor`、世界登录协调、场景状态、AOI 和基础战斗

当前明确不做监控运维、跨服、后台管理、家园系统和完整 MongoDB 落地实现，目标是先把 actor 骨架和核心在线链路迁到 `antares-main` 风格。

## 目录结构

```text
cmd/
  gateway/   # WebSocket 网关入口
  portal/    # 门户服务入口
  player/    # 玩家 actor 节点入口
  world/     # 世界 actor 节点入口

internal/
  cluster/   # actor runtime 与内部消息
  gateway/   # WebSocket 传输层与 channelActor
  player/    # playerActor 与玩家服务
  portal/    # 门户领域、仓储、服务
  world/     # WorldActor、世界服务、仓储
  platform/  # 配置、日志、启动辅助

pkg/
  aoi/       # 独立 AOI 网格实现
  pb/        # Protobuf 生成代码

api/proto/
  portal/    # 门户协议
  world/     # 世界协议
```

## 当前迁移状态

actor 化迁移目前已经完成了第一批骨架工作：

- 已引入 `protoactor-go`
- 已补上 `player` 服务配置、actor runtime 配置和 MongoDB 配置占位
- 已新增 `playerActor`
- 已新增 `WorldActor`
- 已新增 `channelActor`
- 已补上 `channel -> world -> player` 登录链路的单测

当前仍保留的旧实现：

- `internal/gateway/net/ws_server.go` 还在使用旧的 handler 直调路径
- `portal` 仍然是本地账号/令牌服务
- `world` 的场景和战斗逻辑仍然通过现有 service/repository 包提供

也就是说，仓库现在已经进入“actor 骨架已落地，热链路继续迁移中”的阶段。

## 当前能力

### 已完成

- 本地登录账号校验
- 令牌签发与会话绑定
- `channelActor / playerActor / WorldActor` 基础骨架
- `player` 独立进程入口
- 世界登录协调的最小 actor 链路
- 玩家进入场景
- AOI 九宫格可见性计算
- 基础怪物生成
- 基础攻击与伤害结算
- 世界事件广播

### 暂未覆盖

- 完整 MongoDB 数据加载、脏追踪与刷盘
- 独立 actor 节点间远程通讯
- 角色选择与多角色管理
- 技能、Buff、掉落、背包、任务
- 跨服、聊天、公会、邮件
- Prometheus / Grafana / Tracing 等运维组件

## 本地开发

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

健康检查地址：

- `http://127.0.0.1:8082/healthz`：player
- `http://127.0.0.1:8081/healthz`：portal
- `http://127.0.0.1:8083/healthz`：world
- `http://127.0.0.1:8080/healthz`：gateway

## 推荐阅读入口

如果你想快速看懂当前 actor 迁移进度，建议从下面这些文件开始：

- [cmd/gateway/main.go](/D:/workspace/githut_repo/overmind/cmd/gateway/main.go:1)
- [cmd/player/main.go](/D:/workspace/githut_repo/overmind/cmd/player/main.go:1)
- [internal/cluster/messages/messages.go](/D:/workspace/githut_repo/overmind/internal/cluster/messages/messages.go:1)
- [internal/gateway/actor/channel_actor.go](/D:/workspace/githut_repo/overmind/internal/gateway/actor/channel_actor.go:1)
- [internal/player/actor/player_actor.go](/D:/workspace/githut_repo/overmind/internal/player/actor/player_actor.go:1)
- [internal/world/actor/world_actor.go](/D:/workspace/githut_repo/overmind/internal/world/actor/world_actor.go:1)
- [internal/world/service/scene_service.go](/D:/workspace/githut_repo/overmind/internal/world/service/scene_service.go:1)

## 下一步

接下来最直接的几步是：

1. 把 `ws_server` 的登录和业务分发切到 `channelActor`
2. 给 `player/world` 增加远程 actor 节点通讯
3. 把 MongoDB 的数据加载、脏追踪和 flush 模型接进来
4. 让 `portal` 彻底退出第一版热链路，只保留外围入口能力
