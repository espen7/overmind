# Overmind Remote Envelope Design

## Context

`overmind` 当前已经补上了 `channelActor / playerActor / WorldActor` 的本地 actor 骨架，但 actor 之间还没有真正的跨进程远程通信。现阶段：

- 客户端到 `gateway` 的消息仍然是 `Protobuf`
- actor 之间仍然是同进程 Go struct 消息
- 尚未建立统一的远程消息信封模型

下一步如果继续把 `gateway + player + world` 迁到真正的多进程节点，就需要先统一“进程间到底传什么”。

这份设计只解决一件事：

- `gateway / player / world` 之间的远程消息模型

明确不覆盖：

- 具体 cluster provider 选型
- MongoDB 落地细节
- 业务 handler 实现
- GM / admin

## Goal

为 `overmind` 定义一套稳定的、可演进的、适合 `protoactor-go` 远程通信的消息模型，使得：

1. 客户端协议和服务器内部协议边界清晰
2. actor 路由信息统一进入标准信封
3. 远程传输只维护一套序列化体系
4. 后续登录链路和 gameplay 链路都能在同一模型上扩展

## Recommended Approach

### Option A: `Envelope + typed Letter` with Protobuf

所有进程间消息统一放进一个 `Envelope`，`Envelope` 携带服务器内部路由元数据；业务载荷放在 `Letter` 中；`Letter` 再分为：

- `ClientLetter`
- `ServerLetter`
- `ControlLetter`

推荐原因：

- 客户端消息透传和服务器内部协同不会混成一种 payload
- actor 路由元数据和业务数据分层清晰
- 最适合后续加 trace、超时、重试和版本控制
- 与当前 `gateway` 的 `msg_id + payload` 协议边界兼容

### Option B: only `Envelope + raw ClientLetter`

所有远程消息都统一包成客户端原始消息风格，只保留：

- `msg_id`
- `payload`

不推荐原因：

- 服务器内部消息会被迫伪装成客户端协议
- 登录协调、顶号、世界投影同步、节点控制都没有自然表达方式
- 后续消息演进会很快失控

### Option C: JSON metadata + binary payload

元数据用 JSON，业务 payload 单独放二进制。

不推荐原因：

- 会引入第二套编码规则
- 类型系统弱，调试看似方便，长期维护更差
- 不适合高频 actor 热链路

本设计采用 **Option A**。

## Core Design

### 1. `Envelope` is the only remote transport unit

所有进程间通信统一传输 `Envelope`。

`Envelope` 负责：

- 标识消息来源和目标
- 携带跨节点路由所需元数据
- 标识消息是否需要应答
- 标识编码版本和追踪信息
- 承载一个具体 `Letter`

也就是说，远程 actor 不直接传裸 `PlayerLoginReq`、`MoveRequest` 之类对象，而是统一传：

`Envelope(meta + body)`

### 2. `Letter` is the business payload container

`Letter` 不承载路由元数据，只承载真正的业务内容。

它分成三类：

#### `ClientLetter`

用于包装客户端原始协议消息。

适用场景：

- `gateway -> player`
- `gateway -> world`
- 某些需要原样透传客户端协议的链路

结构上保留：

- `msg_id`
- `payload`

其中 `payload` 是客户端 protobuf 消息体。

#### `ServerLetter`

用于服务器内部业务消息。

适用场景：

- `world -> player` 登录协调
- `player -> world` 世界投影同步
- 重连恢复
- 顶号通知
- 业务内部事件投递

这类消息不再强行复用客户端协议编号，而是有自己独立的 protobuf message schema。

#### `ControlLetter`

用于系统控制消息。

适用场景：

- 心跳
- ack
- 超时取消
- passivate
- 节点间连通性探测
- actor 生命周期控制

这样可以把系统控制面和业务面彻底拆开。

## Envelope Metadata

`Envelope` 里的目标 actor 标识采用拆字段方案，而不是单个拼接字符串。

### Required fields

- `trace_id`
- `request_id`
- `from_node`
- `from_actor_kind`
- `from_actor_id`
- `to_actor_kind`
- `to_actor_id`
- `timestamp_ms`
- `codec_version`
- `require_reply`

### Context fields

这些字段按链路需要填写，不要求每次都非空：

- `conn_id`
- `player_id`
- `world_id`
- `session_id`

### Why split actor target fields

采用：

- `to_actor_kind`
- `to_actor_id`

而不是：

- `target = "player/1001"`

原因是：

1. 日志过滤更容易
2. 网关和远程层不用反复拆字符串
3. 后续加指标和分桶更自然
4. 更适合多语言和多节点场景

## Serialization Rules

### One serialization system only

远程通信统一使用 `Protobuf`。

也就是说：

- 客户端协议是 protobuf
- 远程 `Envelope` 也是 protobuf
- `ServerLetter` / `ControlLetter` 也是 protobuf

不再额外引入第二套内部序列化体系。

### Local actor messages stay native

同进程 actor mailbox 内部仍然允许直接传 Go struct。

只有在“跨进程 / 跨节点”边界上，才必须转换为 `Envelope protobuf`。

这样可以兼顾：

- 本地开发和单测的轻量性
- 远程通信的稳定 schema

## Suggested Proto Shape

第一版推荐的结构是：

```protobuf
message Envelope {
  string trace_id = 1;
  string request_id = 2;
  string from_node = 3;
  string from_actor_kind = 4;
  string from_actor_id = 5;
  string to_actor_kind = 6;
  string to_actor_id = 7;
  string conn_id = 8;
  int64 player_id = 9;
  int64 world_id = 10;
  int64 timestamp_ms = 11;
  bool require_reply = 12;
  uint32 codec_version = 13;

  oneof body {
    ClientLetter client = 20;
    ServerLetter server = 21;
    ControlLetter control = 22;
  }
}

message ClientLetter {
  uint32 msg_id = 1;
  bytes payload = 2;
}
```

`ServerLetter` 和 `ControlLetter` 再继续拆 `oneof`：

- `ServerLetter` 放业务内部 protobuf 消息
- `ControlLetter` 放系统控制 protobuf 消息

## Routing Semantics

### Gateway side

`gateway` 负责：

- 解客户端 protobuf
- 根据现有 `msg_id -> actor target` 路由表决定投给 `player` 还是 `world`
- 生成 `Envelope`
- 把客户端消息包装成 `ClientLetter`

### Player / World side

`player` 和 `world` 负责：

- 解开 `Envelope`
- 校验目标是否匹配本 actor
- 根据 `body` 类型决定进入哪个 dispatcher

具体规则：

- `ClientLetter` 进入客户端协议 dispatcher
- `ServerLetter` 进入内部消息 dispatcher
- `ControlLetter` 进入系统控制 dispatcher

## Error Handling

### Invalid envelope

如果 `Envelope` 缺少关键目标字段，直接拒收并记录结构化错误日志。

### Unknown actor target

如果 `to_actor_kind` 无法识别，进入 dead-letter 或错误回执逻辑，不允许静默吞掉。

### Unknown body type

如果 `body` 为空或类型未知，按协议错误处理。

### Version mismatch

`codec_version` 不匹配时：

- 可以拒绝
- 也可以进入兼容分支

但必须明确记录，不能隐式忽略。

## Migration Plan

这一设计只要求先覆盖第一段链路：

1. `gateway -> world` 登录请求
2. `world -> player` 登录协调请求
3. `player -> world` 登录响应

之后再扩到：

4. `gateway -> player` 玩家私有业务消息
5. `gateway -> world` 世界业务消息
6. `player <-> world` 世界投影和业务协同

这样可以避免一次性把全部 gameplay 协议都卷进远程迁移。

## Non-Goals

当前不做：

- 把所有现有本地 Go struct 消息立刻全部 protobuf 化
- 为每一种本地内部消息都生成远程消息定义
- 设计复杂的消息重放、事务补偿和跨节点一致性机制

## Risks and Mitigations

### Risk: `Envelope` 过重

Mitigation:

元数据字段只保留真正会参与路由、诊断和兼容控制的字段，不把业务字段塞到 envelope 顶层。

### Risk: 客户端协议和服务器内部协议耦合

Mitigation:

通过 `ClientLetter / ServerLetter / ControlLetter` 分层，避免所有消息都退化成 `msg_id + payload`。

### Risk: 远程和本地两套消息模型割裂

Mitigation:

明确规定：

- 本地 actor 可继续用 Go struct
- 远程边界统一转换为 `Envelope protobuf`

只在边界转换，不在业务内部重复做适配。

## Approval Baseline

这份设计基于本轮已确认的共识：

- 远程通信建议继续参考 `antares-main` 的边界设计
- 进程间消息体采用 `Envelope`
- `Envelope` 包装 `Letter`
- `Letter` 中保留客户端原始 `msg_id + payload`
- 但不把客户端原始消息设计成唯一远程消息体
- 目标 actor 标识采用拆字段：
  - `to_actor_kind`
  - `to_actor_id`
