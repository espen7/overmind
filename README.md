# SLG Game Server Infrastructure Blueprint
# SLG 游戏服务器底层基础建设与路由设计蓝图 (最终版)

本蓝图专注于游戏服务器的**底层网络、连接管理、网关路由、跨进程通信以及集群架构**，不涉及任何游戏具体业务。此版本已完全同步通过交互确认的各项决策，采用 **“全 Actor 闭环架构”**，整个集群的通信、断线清理、以及多播事件广播（聊天、大地图 AOI）全部使用 `ergo` 原生机制实现。

---

## 1. 基础建设技术栈与决策决策矩阵

根据设计讨论，本项目的底层核心技术栈及决策如下：

| 维度                | 决策方案                             | 实施细节与理由                                                                                                                                            |
|:------------------|:---------------------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------|
| **客户端长连接**        | **WebSocket (裸连接)**              | 开发调试阶段直接使用裸 WS 协议。后续生产环境通过 Nginx 挂载 SSL 证书统一提供 **WSS** 支持，业务层无需编写证书及加解密逻辑。                                                                         |
| **集群注册与发现**       | **ergo 原生注册器 (gen.Registrar)**   | **不依赖外部 etcd/Consul**。开发环境使用 `ergo` 原生 UDP 多播发现连接；生产环境使用静态路由注册器 (`ergo.services/static`) 或自研原生注册节点，贯彻零依赖哲学。                                        |
| **网关连接模型**        | **Channel Actor (每个连接一个 Actor)** | **全 Actor 闭环**。网关为每个 Socket 派生一个 `Channel Actor`。通过 `ergo` 的 **Link (双向生死绑定)** 机制，Socket 断开时自动促使后端 `Player Actor` 退出，完全消除下线清理逻辑。                   |
| **网关到节点路由**       | **一致性哈希 (Consistent Hashing)**   | `Channel Actor` 收到客户端请求后，通过 `PlayerID` 进行本地一致性哈希计算，直接定位对应的 `Home` 节点并投递消息。网关本地无状态，不需要查数据库或 Redis 即可完成路由分发。                                         |
| **玩家 Actor 生命周期** | **惰性加载 + 超时自动销毁**                | 消息第一次路由到节点时，如果内存中没有该玩家，则自动触发 DB 读取并 Spawn Actor；若玩家持续 10 分钟没有消息交互，则自动数据写回 DB 并销毁 Actor，释放内存。                                                       |
| **数据持久化策略**       | **混合策略 (同步+异步写回)**               | 充值、抽卡、商城购买等高价值操作采用**同步写入 (Write-Through)**；常规建筑升级、资源变动等操作在内存中即时修改并返回，每 5 分钟**异步批量写回 (Write-Back)**。                                                |
| **大地图加载策略**       | **空间 Cell 惰性加载 + Chunk 级读写**     | 大地图划分成多个 `Cell`，Cell 内部由 `Chunk` 组成。Cell Actor 仅在所属 Chunk 活跃时，从 DB 懒加载对应的 `Chunk` 动态状态；闲置时按 Chunk 刷入 DB 并卸载。                                       |
| **地图视野同步 (AOI)**  | **ergo 原生分布式事件总线 (Pub/Sub)**     | **不需手写广播过滤**。每个 `Cell Actor` 为其管理的 Chunk 注册一个 `ergo` 原生事件（Event）。网关 `Channel Actor` 订阅视口覆盖的 Chunk Event。Cell 内有变动时直接 `Publish`，由 `ergo` 底层自动跨节点多播。 |
| **状态同步机制**        | **增量脏同步 + AOI 事件推送**             | 个人数据使用**脏标记 (Dirty Flags)** 机制，在请求结束时统一打包增量变更（命名为 **S2C_SyncData**）推送给客户端；大地图数据由 `Cell Actor` 触发变更，网关按 Chunk 订阅列表多播分发。                             |
| **高并发定时器**        | **本地时间轮 + Redis ZSet 备份**        | 每个逻辑节点独立运行内存分层时间轮（Timing Wheel）驱动定时器。关键事件（如行军到达）同步备份在外部 Redis 的 `ZSet` 中，防止宕机丢失。                                                                   |

---

## 2. 客户端 ↔ 网关 通信协议与包体设计

为了防止粘包、半包、恶意大包攻击，客户端与网关之间采用 **Header-Length 二进制流格式**：

```text
┌───────────────────┬───────────────────┬───────────────────┬───────────────────┐
│ Length (4 bytes)  │  ProtoID (4 bytes)│   SeqID (4 bytes) │  Payload (Bytes)  │
├───────────────────┼───────────────────┼───────────────────┼───────────────────┤
│    整个包的长度     │    Protocol ID    │   客户端包序列号   │   Protobuf 数据   │
└───────────────────┴───────────────────┴───────────────────┴───────────────────┘
```

*   **Length (4字节)**：标识紧随其后的包体总长度。网关读取时，先读 4 字节，若超过设定的最大包大小（如 64KB），则判定为恶意攻击，直接断开连接。
*   **ProtoID (4字节)**：协议号。用于标识该消息是什么业务协议，并决定网关将其路由到哪个节点。
*   **SeqID (4字节)**：递增的序列号。用于网关进行防重放攻击、防作弊、丢包重传校验。
*   **Payload**：由对应 `ProtoID` 定义的 Protobuf 序列化二进制数据。

---

## 3. 网关连接管理与 Session 设计

网关为每一个客户端连接 Spawn 一个本地的 `Channel Actor`。

### 3.1 Channel Actor 状态数据 (State)
每个 `Channel Actor` 在内存中维护该连接的所有 Session 状态：

```go
type ChannelState struct {
    ConnID     int64          // 连接唯一ID
    PlayerID   string         // 玩家ID（未登录前为空）
    WSConn     *websocket.Conn// 底层 WebSocket 连接对象
    HomePID    gen.PID        // 该连接对应的后端 Home 服 Player Actor PID
    WorldPID   gen.PID        // 该连接对应的后端 World 服 Cell Actor PID
    LastHeart  time.Time      // 上次收到心跳的时间
    AuthStatus bool           // 是否通过 Token 认证
}
```

---

## 4. 网关 ↔ 后端节点路由机制 (Routing Logic)

由于采用了全 Actor 闭环架构，跨进程的路由、封包、网络传输完全由 `ergo` 的消息总线接管，业务层无需编写任何网络路由连接代码。

### 4.1 协议号范围划分 (Protocol Partitioning)
网关解析客户端发来的包头中的 `ProtoID`，根据区间直接路由：

```text
[ProtoID 范围]       ──────►  [路由去向]
1000 - 9999         ──────►  Gate 本地处理 (如 Login, Heartbeat, Reconnect)
10000 - 19999       ──────►  Home 逻辑服 (个人数据, 建筑升级, 抽卡)
20000 - 29999       ──────►  World 地图服 (大地图行军, 采集, 侦察)
30000 - 39999       ──────►  Alliance 联盟服 (联盟帮助, 集结战争)
```

### 4.2 路由流程演示：玩家请求建筑升级 (`ProtoID: 10005`)

```mermaid
sequenceDiagram
    participant Client as 客户端
    participant ChannelActor as 网关 Channel Actor
    participant PlayerActor as 逻辑服 Player Actor

    Client->>ChannelActor: 发送包 (ProtoID: 10005, Payload: BuildUpgradeReq)
    Note over ChannelActor: 1. 读取包头，识别 ProtoID 为 10005<br/>2. 根据一致性哈希计算出对应的 Home 节点 NodeName<br/>3. 获取该玩家的 HomePID
    ChannelActor->>PlayerActor: ctx.Send(HomePID, RPCEnvelope{ProtoID: 10005, Payload: bytes})
    Note over PlayerActor: 1. 收到 RPCEnvelope 消息<br/>2. 发现本地没有该 Actor，触发 DB 懒加载并 Spawn<br/>3. 将 ChannelActor 的 PID 绑定为数据推送目标 (Link 绑定)<br/>4. 执行升级逻辑
    PlayerActor->>ChannelActor: ctx.Send(GatePID, S2C_SyncData)
    ChannelActor->>Client: 将消息写回 WebSocket 物理 Socket
```

---

## 5. 数据状态同步机制 (State Synchronization)

为了确保高吞吐量与极低带宽消耗，游戏状态同步采取以下两种不同设计：

### 5.1 个人内政增量脏同步 (Home ↔ Client)
*   **脏标记存储**：玩家 Actor (Player Actor) 在内存中维护各个子模块的数据版本号或 `isDirty` 标记。
*   **同步协议 (S2C_SyncData)**：
    1.  当玩家请求某操作（如扣除资源、消耗道具）时，Player Actor 内部修改对应模块数据，并设置 `module.isDirty = true`。
    2.  在当前消息处理器（`HandleCall` 或 `HandleCast`）返回之前，Player Actor 执行一个公共拦截函数：
        *   检查所有子模块的 `isDirty` 标记。
        *   将所有标记为脏的模块的增量更新（例如：`Gold: 1000`，`Item[102]: count-1`）统一打包进一个 **`S2C_SyncData`** 消息包。
        *   通过 `ctx.Send(GatePID, S2C_SyncData)` 发送给网关。
        *   网关的 `Channel Actor` 收到后写回 Socket。

### 5.2 大地图 AOI 区块多播推送 (Cell ↔ Gate ↔ Client)
*   **大地图空间划分 (类似 BigWorld)**：
    *   **Map (全图)**：例如 1200x1200 格。
    *   **Cell (计算边界)**：由 `Cell Actor` 驱动（线程与协程隔离边界），面积较大（如 240x240 格）。
    *   **Chunk (加载与广播边界)**：逻辑分块（如 30x30 格）。
    *   **Tile (格子)**：大地图上最小的 2D 坐标单位（1x1），存储具体地形和实体指针。
*   **基于 ergo 原生事件总线 (Pub/Sub) 的视野广播机制**：
    1.  `Cell Actor` 启动时，为其管理的每个活跃 Chunk 注册一个 `ergo` 原生事件（例如调用 `ctx.RegisterEvent(eventChunkID, options)`）。
    2.  当玩家滑动屏幕，客户端发出视角移动消息时，网关的 `Channel Actor` 计算出当前覆盖的多个 Chunk ID，并调用 `ctx.LinkEvent(CellPID, eventChunkID)` 来订阅这些区块。当视角移开时，调用 `ctx.UnlinkEvent` 退订。
    3.  当 Chunk 内发生任何属性修改或行军变动时，`Cell Actor` 直接将事件数据 `Publish` 广播到对应的 `eventChunkID` 上。
    4.  `ergo` 底层网络层会自动检索并跨节点，把该事件多播分发给所有订阅了此事件的网关 `Channel Actor`，再由其写回客户端。
*   **优势**：网关层不用编写广播过滤代码，业务层（Cell Actor）不需要知道具体连接，完美解耦，全部由 `ergo` 高性能多播分发。

---

## 6. 跨节点 RPC RPCEnvelope 定义

为了实现高性能且类型安全的跨节点通信，我们定义一个统一的 RPC 传输外壳。

```protobuf
syntax = "proto3";
package api;

// 跨物理进程传输的消息外壳
message RPCEnvelope {
    int64  session_id = 1; // 网关的 Session ID
    string player_id  = 2; // 玩家唯一 ID
    int32  proto_id   = 3; // 实际业务协议号
    bytes  payload    = 4; // 实际业务协议的二进制数据（如 BuildUpgradeReq）
}
```

---

## 7. 基于 golang-standards/project-layout 规范的脚手架设计

为确保项目的标准化，防止代码重构混乱，项目目录架构严格按照官方规范进行如下规划：

```text
overmind/
├── api/                   # [标准] 存放接口定义文件与生成的客户端/服务端存根
│   ├── rpc/               # 内部节点 RPC 协议 (.proto 协议及生成的 Go 代码)
│   │   ├── rpc.proto
│   │   └── rpc.pb.go
│   └── game/              # 客户端 ↔ 服务端 协议 (.proto 及生成的 Go 代码)
│       └── game.proto
│
├── cmd/                   # [标准] 主入口目录。每个子文件夹编译后产生对应的可执行文件
│   ├── gate/              # 网关服主启动器
│   │   └── main.go
│   ├── home/              # 玩家逻辑服主启动器
│   │   └── main.go
│   └── world/             # 大地图服主启动器
│       └── main.go
│
├── internal/              # [标准] 私有库代码，存放不希望被外部项目导入的核心代码
│   ├── app/               # 各节点专属的内部运行逻辑与 Actor 行为实现
│   │   ├── gate/          # 网关的 Connection 循环和一阶路由逻辑
│   │   ├── home/          # Player Actor 行为与 Gorm/MySQL 数据库交互实现
│   │   └── world/         # Cell Actor 及大地图空间行军、AOI 广播逻辑
│   │
│   └── pkg/               # 各节点共享的私有基础库 (框架 SDK 所在地)
│       ├── actor/         # 基于 ergo 的 Actor 行为包装与 RPC 传输外壳收发辅助
│       ├── network/       # TCP/WS 网关底层网络封装、Length-Header 封包与 Session 管理
│       ├── timer/         # 高性能分层时间轮 (Timing Wheel) 实现
│       └── chunk/         # 地图 Chunk 序列化及数据存储加载引擎
│
├── configs/               # [标准] 存放各节点的配置文件（如 YAML / JSON）
│   ├── gate.yaml
│   ├── home.yaml
│   └── world.yaml
│
├── scripts/               # [标准] 编译、自动代码生成 (如 protoc-gen-go) 等脚本文件
│   └── gen_proto.sh
│
├── go.mod
└── Makefile
```
