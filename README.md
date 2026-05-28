# Overmind

`overmind` 是一个用 Go 编写的 SLG 游戏服务器。当前分支参考了 `mengfangtan/mmo-game-server` 的服务边界，但没有照搬完整基础设施，而是先把最核心的实时玩法链路重建出来。

## 当前目标

这一版聚焦三个服务：

- `gateway`：负责 WebSocket 接入、二进制包编解码、会话绑定和消息分发
- `portal`：负责登录校验、令牌签发，以及玩家进入世界前的基础上下文
- `world`：负责场景状态、AOI 可见性、怪物管理和基础战斗

当前明确不做监控运维、跨服、后台管理、家园系统和数据库持久化扩展，目标是先把“登录 -> 进图 -> 移动 -> 攻击 -> 广播”这条主链路跑通。

## 目录结构

```text
cmd/
  gateway/   # WebSocket 网关入口
  portal/    # 门户服务入口
  world/     # 世界服务入口

internal/
  gateway/   # 传输层、包协议、会话
  portal/    # 门户领域、仓储、服务、传输适配
  world/     # 世界领域、仓储、场景服务、战斗服务
  platform/  # 配置、日志、启动辅助

pkg/
  aoi/       # 独立 AOI 网格实现
  pb/        # Protobuf 生成代码

api/proto/
  portal/    # 门户协议
  world/     # 世界协议
```

## 当前运行方式

虽然代码已经拆成了 `gateway / portal / world` 三个服务边界，但**内部通讯目前还是进程内直接调用**，还没有接入 gRPC、NATS 或其他 RPC/MQ 方案。

当前流程是：

1. 客户端连接 `gateway`
2. `gateway` 解包请求并调用 `portal` handler 完成登录
3. 登录成功后，`gateway` 把角色上下文绑定到会话
4. 进入场景、移动、攻击等消息由 `gateway` 直接调用 `world` handler
5. `world` 计算 AOI 和战斗结果，再由 `gateway` 回推给可见玩家

这样的好处是：

- 服务职责已经清晰
- 不依赖额外基础设施就能跑通主链路
- 后续替换为真正的服务间通讯时，领域逻辑不需要重写

## 当前能力

### 已完成

- 本地登录账号校验
- 令牌签发与会话绑定
- 玩家进入场景
- AOI 九宫格可见性计算
- 基础怪物生成
- 基础攻击与伤害结算
- 世界事件广播

### 暂未覆盖

- 数据库持久化
- 角色选择与多角色管理
- 技能、Buff、掉落、背包、任务
- 跨服、聊天、公会、邮件
- Prometheus / Grafana / Tracing 等运维组件

## 本地开发

### 生成 Protobuf

```powershell
.\scripts\proto_gen.bat
```

### 运行测试

```powershell
go test ./...
```

### 构建服务

```powershell
go build ./cmd/gateway ./cmd/portal ./cmd/world
```

### 启动服务

```powershell
go run ./cmd/portal
go run ./cmd/world
go run ./cmd/gateway
```

健康检查地址：

- `http://127.0.0.1:8081/healthz`：portal
- `http://127.0.0.1:8082/healthz`：world
- `http://127.0.0.1:8080/healthz`：gateway

## 推荐阅读入口

如果你想快速看懂当前实现，建议从下面几个文件开始：

- [cmd/gateway/main.go](/D:/workspace/githut_repo/overmind/cmd/gateway/main.go:1)
- [internal/gateway/net/ws_server.go](/D:/workspace/githut_repo/overmind/internal/gateway/net/ws_server.go:1)
- [internal/portal/service/portal_service.go](/D:/workspace/githut_repo/overmind/internal/portal/service/portal_service.go:1)
- [internal/world/service/scene_service.go](/D:/workspace/githut_repo/overmind/internal/world/service/scene_service.go:1)
- [internal/world/service/combat_service.go](/D:/workspace/githut_repo/overmind/internal/world/service/combat_service.go:1)
- [pkg/aoi/grid.go](/D:/workspace/githut_repo/overmind/pkg/aoi/grid.go:1)

## 后续演进建议

下一步比较自然的方向有三条：

1. 把 `gateway -> portal/world` 的进程内调用替换成真正的 RPC
2. 为 `world` 引入更完整的实体生命周期和事件流
3. 为 `portal` 增加角色列表、重连、分线/选服等入口能力
