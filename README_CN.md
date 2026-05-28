# Overmind

一个使用 Go 编写的 SLG 游戏服务器。

这个分支参考了 `mengfangtan/mmo-game-server` 的核心拆分方式，但只保留首阶段真正需要的能力，明确不包含监控和运维体系。

## 第一阶段范围

- `gateway`：WebSocket 接入、消息包编解码、会话绑定
- `portal`：登录入口、令牌签发、进图前上下文分发
- `world`：场景进入、AOI 可见性、基础战斗

## 当前目录

```text
cmd/
  portal/    # 门户服务入口
  world/     # 世界服务入口
  gateway/   # WebSocket 网关入口

internal/
  portal/    # 门户领域、仓储、服务、传输层
  world/     # 场景、战斗、世界状态、传输层
  gateway/   # WebSocket 传输与二进制包协议
  platform/  # 配置、日志、启动辅助

pkg/
  aoi/       # 独立 AOI 网格实现
  pb/        # Protobuf 生成代码

api/proto/
  portal/    # 门户协议
  world/     # 场景与战斗协议
```

## 当前明确不做

- 监控、告警、Tracing、运维面板
- 跨服通信、服务发现、集群编排
- 家园/主城玩法、后台管理、GM 工具
- 依赖数据库的完整持久化成长体系

## 本地开发

### 生成 protobuf

```powershell
.\scripts\proto_gen.bat
```

### 运行测试

```powershell
go test ./...
```

### 启动服务

```powershell
go run ./cmd/portal
go run ./cmd/world
go run ./cmd/gateway
```

健康检查地址：

- `http://127.0.0.1:8081/healthz`
- `http://127.0.0.1:8082/healthz`
- `http://127.0.0.1:8080/healthz`

## 说明

当前实现保持了服务边界，但仓储仍然是内存版，服务之间也以本地进程内接线为主，目标是先把登录、进图、AOI 和基础战斗链路稳定跑通。
