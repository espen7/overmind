# Overmind 消息分层设计

## 背景

当前仓库同时存在客户端网络包、服务间 `Envelope`、以及 `channelActor` 进程内本地消息三种通信形态。
如果这些概念命名不清晰，就很容易出现：

- 把客户端 `Packet` 和服务间 `Envelope` 当成同一种具体结构
- 把进程内 mailbox 消息误叫成 `Envelope` 或 `Frame`
- 后续扩展 `player -> world`、`player -> player` 业务消息时继续混用本地 struct 和跨进程协议

因此这里明确把消息系统分成三层。

## 目标

- 明确区分 `client <-> gateway`、`process <-> process`、`in-process actor mailbox`
- 把跨进程通信统一收口到 protobuf `Envelope`
- 把进程内消息改成 `Command / Event` 命名，避免和 wire envelope 混淆
- 允许在不打断当前重构节奏的前提下逐步演进

## 分层模型

### 1. ClientPacket

`ClientPacket` 是 `client <-> gateway` 的客户端传输信封。

职责：

- 表达客户端协议号 `msg_type`
- 表达二进制 payload
- 解决拆包、粘包、长度边界

不负责：

- 服务发现
- actor identity 路由
- trace / request 元数据
- 服务间超时和重试

当前代码位置：

- [internal/gateway/protocol/packet.go](/D:/workspace/githut_repo/overmind/internal/gateway/protocol/packet.go:1)

### 2. RpcEnvelope

`RpcEnvelope` 是 `process <-> process` 的统一 RPC 信封。
当前具体实现就是 [api/proto/kit/envelope.proto](/D:/workspace/githut_repo/overmind/api/proto/kit/envelope.proto:1) 里的 `Envelope`。

它覆盖：

- `gateway -> player`
- `gateway -> world`
- `world -> player`
- `player -> world`
- `player -> player`

职责：

- 承载 trace / timestamp / sender / target 等服务间元数据
- 承载客户端语义载荷 `EdgeLetter`
- 承载内部系统语义载荷 `MeshLetter`

约束：

- 进程间通信一律走 protobuf `Envelope`
- 不再跨进程直接发送 Go struct
- 不把本地 mailbox 命令误包装成“新的 envelope 类型”

### 3. LocalCommand / LocalEvent

`LocalCommand / LocalEvent` 是 actor 进程内 mailbox 消息。

这层只用于：

- 同进程内 actor 间投递
- actor 自己的初始化、状态切换、局部路由

这层不需要：

- protobuf 序列化
- trace 元数据
- 远程传输兼容

因此本地消息继续允许使用 Go struct，但命名必须清晰表达它是本地命令，而不是 wire envelope。

## 命名规则

### 客户端传输层

- 使用 `ClientPacket`
- 不再把它叫成泛化的 `Packet` 作为长期主命名
- 当前为了兼容旧调用点，代码里保留 `type Packet = ClientPacket`

### 服务间 RPC 层

- 抽象概念：`RpcEnvelope`
- 当前具体 proto 名称：`Envelope`
- 具体载荷：
  - `EdgeLetter`
  - `MeshLetter`

说明：

- 这里不按 `gate-player`、`player-world` 再拆很多种 envelope
- 差异放在 letter/body 类型，而不是 envelope 数量

### 进程内 mailbox 层

- 使用 `*Command` / `*Event`
- 不再使用 `Frame`
- 不再使用名字里带 `Envelope`、但实际不是 protobuf envelope 的本地 struct

当前已落地的例子：

- `LoginCommand`
- `RouteToPlayerCommand`
- `RouteToWorldCommand`

位置：

- [internal/gateway/actor/channel_actor.go](/D:/workspace/githut_repo/overmind/internal/gateway/actor/channel_actor.go:1)

## Proxy 与 Envelope 的关系

`proxy` 不是 envelope。

- `ClientPacket` 是客户端传输信封
- `Envelope` 是服务间 RPC 信封
- `player/world proxy` 是基于 `kind + identity` 找实体并投递 `Envelope` 的远程代理

也就是说：

- `gateway` 是 cluster client
- `gateway` 通过 `player/world shard proxy` 把 `Envelope` 发给目标实体
- `player` 通过 `world/player shard proxy` 把 `Envelope` 发给其他实体

## 当前代码状态

已经落地：

- `ClientPacket` 命名与兼容别名
- `Envelope` 作为统一的进程间 RPC 信封
- `channelActor` 本地消息改成 `*Command`

尚未完全落地：

- `ws_server` 主链路还没有完全切进 `channelActor`
- `EdgeLetter` 还没有成为所有客户端业务消息进入内部总线的统一承载体
- 更多 `player -> world` / `player -> player` 业务消息仍待补齐

## 后续演进建议

1. 把 `ws_server` 的登录与业务分发切到 `channelActor`
2. 让客户端消息统一从 `ClientPacket -> EdgeLetter -> RpcEnvelope`
3. 保持所有跨进程链路只传 `Envelope`
4. 本地 actor 消息只保留 `Command / Event`
