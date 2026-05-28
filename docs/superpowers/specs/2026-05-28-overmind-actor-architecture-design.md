# Overmind Actor Architecture Design

## Context

`overmind` 当前已经重写出 `gateway + portal + world` 的最小骨架，但内部通信仍然是进程内直接调用，数据层也还是纯内存仓储。接下来要把内部通信、玩家状态承载和数据落地整体切到更接近 `antares-main` 的设计。

这份设计只覆盖三件事：

1. actor 划分与节点边界
2. 数据加载、内存态管理与落地
3. 节点间 RPC / 消息通信方式

明确排除：

- GM
- 后台 admin
- 监控运维
- 第一版 `world` 内部更细粒度的 region/cell actor 拆分

## Decisions

本设计将后续未特别说明的实现细节默认收敛到 `antares-main` 风格，已确认的约束如下：

1. 采用四节点边界：`gateway + portal + player + world`
2. `player` 是独立进程节点，不作为 `world` 的内部子模块
3. `world` 是独立进程节点，第一版采用 `1 worldId = 1 WorldActor`
4. 玩家私有主数据以 `player` 为唯一真源
5. `world` 只维护世界运行需要的玩家投影与世界态
6. 数据存储底座选择 `MongoDB`
7. 未特别指定的登录与消息路由细节，优先参考 `antares-main`

## Recommended Approach

### Option A: Full `antares-main` hot-path alignment

核心热链路尽量贴近 `antares-main`：

- 客户端直接连 `gateway`
- `gateway` 为每个连接创建 `channelActor`
- `channelActor` 根据协议路由表，把消息直投 `playerActor` 或 `worldActor`
- `world` 负责首段登录接入和世界会话建立
- `player` 负责玩家主数据与玩家级业务

推荐原因：

- 与目标参考实现最一致，后续迁移心智成本最低
- actor 职责清晰，连接态 / 玩家态 / 世界态不会混在一层
- 最适合后续引入 `protoactor-go` cluster identity / partition

### Option B: Keep `portal` on the login hot path

保留 `portal -> gateway -> actor cluster` 的前置入口，由 `portal` 完成认证、选服、签发进入世界票据，再进入 actor 集群。

为什么不推荐作为第一版：

- 偏离 `antares-main` 的核心登录链路
- 会把连接入口和 actor 入口拆成两段，增加状态同步复杂度
- 很容易在 `portal` 中重新堆出一层“半个玩家状态机”

### Option C: Actor only inside `world`

只把 `world` 做成 actor 化，`player` 继续保持普通服务或仓储层。

为什么不推荐：

- 不符合“玩家私有数据以 `player` 为真源”的已确认约束
- 玩家在线状态、顶号、断线重连、个人定时器会被塞回 `world`
- 后续会更难做玩家迁移、数据卸载与隔离压测

本设计采用 **Option A**，同时保留一个兼容性处理：

- `portal` 服务不删除，但不进入第一版 actor 热链路
- `portal` 作为后续账号平台、SDK 登录、公告、服列表、预登录票据的轻量入口预留
- 第一版本地开发和核心 actor 链路默认按 `antares-main` 风格直连 `gateway`

## Target Topology

### `gateway`

职责：

- 接收 TCP / WebSocket 连接
- 为每个客户端连接创建 `channelActor`
- 维护加解密、心跳、连接超时、断线关闭
- 根据协议路由表把客户端消息投递到 `player` 或 `world`
- 把服务端消息回写给客户端

不负责：

- 玩家主数据
- 世界运行态
- 业务落库

### `portal`

职责：

- 作为轻量账号入口预留
- 后续可承载账号认证、SDK 登录、服列表、公告、预登录票据
- 不承载长期玩家状态

第一版定位：

- 保留进程入口与配置边界
- 不纳入核心 actor 登录热链路
- 不参与玩家主状态与世界状态同步

### `player`

职责：

- 承载 `playerActor`
- 以 `playerId` 为 actor identity / shard key
- 维护玩家私有主数据和玩家级业务状态
- 管理在线绑定、顶号、断线、重连、个人定时器
- 对外提供玩家业务校验、扣减、回滚、快照响应

它持有的数据类型包括：

- 玩家基础资料
- 背包
- 建筑/科技/任务
- 邮件
- 个人部队编成
- 个人 action / timer / version

### `world`

职责：

- 承载 `WorldActor`
- 以 `worldId` 为 actor identity / shard key
- 维护世界公共运行态和玩家世界投影
- 管理世界会话、地图广播、行军、驻防、资源点、战斗执行
- 驱动世界级 tick 和世界级脏数据落地

它持有的数据类型包括：

- `playerAbstract` 一类世界投影
- 世界地图实体
- 行军和驻防状态
- 世界 action / version
- 世界广播订阅与 session 索引

第一版 `world` 内部不再继续拆 `regionActor`、`cellActor` 或 `mapActor`。

## Actor Model

### `channelActor`

每个客户端连接对应一个 `channelActor`，生命周期与连接绑定。

职责：

- 维护连接态和鉴权态
- 管理握手、心跳、收发包、超时关闭
- 保存 `playerId/worldId` 等连接上下文
- 根据协议路由表将消息直投 `playerActor` 或 `WorldActor`
- 处理服务端回包和订阅类通知

它是短生命周期 actor，不持久化业务数据。

### `playerActor`

每个玩家对应一个 `playerActor`，以 `playerId` 为唯一身份。

职责：

- 初始化并加载该玩家的全部主数据
- 绑定 / 替换当前在线 `channelActor`
- 处理顶号、多端登录、断线重连
- 处理玩家私有业务
- 必要时向 `world` 发起请求或通知
- 在空闲时进入 passivation，在卸载前 flush 自身脏数据

它是玩家数据和玩家级业务状态机的唯一写入口。

### `WorldActor`

每个世界对应一个 `WorldActor`，以 `worldId` 为唯一身份。

职责：

- 初始化并加载该世界的运行态
- 维护世界 session 表：`playerId -> channelActor`
- 接收来自 `gateway` 的世界消息
- 执行世界逻辑、广播与地图状态更新
- 在需要玩家私有数据时向 `playerActor` 发消息
- 在卸载前 flush 世界脏数据

第一版世界内的高并发仍由单 `WorldActor` 内部的内存态和消息串行化保证，不做更细 actor 划分。

## Login and Message Flow

第一版热链路按 `antares-main` 收敛为：

1. 客户端连接 `gateway`
2. `gateway` 创建 `channelActor`
3. 客户端发送登录请求
4. `channelActor` 把首个登录请求投递给目标 `WorldActor`
5. `WorldActor` 检查或创建世界侧 `playerAbstract` 与 `session`
6. `WorldActor` 通过 request-response 向 `playerActor` 发起 `PlayerLoginReq` 或 `PlayerCreateReq`
7. `playerActor` 加载玩家主数据、绑定 `channelActor`、返回登录结果
8. `channelActor` 完成鉴权态切换，开始接收正常业务消息
9. 后续客户端协议按路由表直接投给 `playerActor` 或 `WorldActor`

这意味着第一版默认不是 `portal -> gateway -> player/world` 的双入口模式，而是 actor 热链路优先贴近 `antares-main`。

## Routing Strategy

`gateway` 维护一份协议到目标 actor 的静态路由表，类似 `antares-main` 中 `Forward.PlayerActor / Forward.WorldActor / Forward.ChannelActor` 的概念。

路由原则：

- 连接控制、心跳、订阅控制：`channelActor`
- 玩家私有业务：`playerActor`
- 世界地图、行军、驻防、战斗、广播：`WorldActor`

这样做的原因是：

- 避免所有请求都串行穿过 `player`
- 保持 `gateway` 路由足够薄
- 让协议边界直接映射业务归属

## RPC and Node Communication

内部通信不采用第一版 gRPC 热链路，而是采用 actor 消息通信，贴近 `antares-main` 的 `tell / ask + sharding proxy` 模式。

在 `protoactor-go` 下对应的实现方向是：

- `gateway` 持有 `player`、`world` 的 cluster identity / PID 路由入口
- 默认使用异步 fire-and-forget 消息
- 只有登录、创建、关键校验这类需要结果的流程使用 request-response
- 不允许 `player/world` 直接共享内存或直接操作对方数据库

通信规则：

1. `gateway -> player/world` 使用 actor 路由
2. `player <-> world` 使用 actor 路由
3. `portal -> actor cluster` 只走非热链路 RPC 或入口消息，不参与高频 gameplay
4. 不引入 HTTP/gRPC 作为玩家在线消息热链路

## Data Ownership

### `player` is the source of truth

以下数据只在 `player` 侧作为真源持有并落地：

- 玩家主档
- 背包
- 建筑与科技
- 任务与邮件
- 部队编成
- 个人 action / version / timer

### `world` keeps projections

以下数据只作为世界投影持有：

- 玩家在世界中的基础展示信息
- 主城坐标、外观、联盟展示信息
- 行军队列中的玩家摘要
- 驻防与战斗执行时所需的裁剪视图

`world` 不直接修改玩家主档，不成为玩家私有数据真源。

## Data Loading and Persistence

数据加载与落地机制直接参考 `antares-main` 的 `DataManager + TraceableMemData` 思路，但用 Go 实现。

### Actor initialization

`playerActor` / `WorldActor` 在首次收到消息时进入 `initializing` 状态：

- 并行加载自己负责的 `MemData`
- 加载完成后切到 `active`
- 加载失败则中止 actor，并让消息调用方收到失败结果

### In-memory runtime

actor 运行时只修改内存对象，不在业务逻辑里即时写库。

### Dirty tracking

每个 actor 持有自己的 `DataManager`：

- `DataManager` 维护多个 `MemData`
- 其中可追踪脏数据的集合实现 `TraceableMemData`
- actor tick 时调用 `traceEntities`
- 框架层负责比较对象快照并生成增量更新

### Flush and passivation

当 actor 进入卸载流程时：

- 停止接收新的业务消息
- 持续 flush 脏数据
- 全部刷盘成功后退出

第一版强约束：

- `playerActor` 卸载前必须完成玩家主数据 flush
- `WorldActor` 卸载前必须完成世界数据 flush
- 不允许“未刷完直接 stop”

## MongoDB Model

第一版 MongoDB 采用“聚合文档 + action/version 文档”的组织方式，贴近 `antares-main`：

- `player`
- `player_action`
- `player_projection` 或 `player_abstract`
- `world_action`
- 视具体玩法补充 `world_march`、`world_city`、`world_resource_node`

设计原则：

- 私有聚合优先归 `player`
- 世界实体优先归 `world`
- 共享查询所需数据优先通过投影维护，而不是双向查主表

## Session and Reconnect Model

会话模型按 `antares-main` 的双层方式处理：

- `playerActor` 持有“当前在线绑定的 `channelActor`”
- `WorldActor` 持有“当前世界 session 表”

登录或重连时：

- 新 `channelActor` 到达后替换旧绑定
- 旧连接收到连接过期消息后关闭
- `playerActor` 负责玩家在线身份唯一性
- `WorldActor` 负责世界广播回包的送达地址

这样可以避免：

- `world` 直接持有完整玩家在线状态
- `gateway` 自己成为玩家状态真源

## Error Handling

- actor 初始化失败：返回失败结果并停止当前 actor
- `player/world` request-response 超时：视为节点不可用或 actor 卡住，返回显式错误
- flush 失败：actor 保持在 stopping 状态重试，不直接丢数据
- 找不到世界 session：丢弃该次世界消息并记录警告
- 连接断开：`channelActor` 停止，`playerActor` 收到 watch terminated 后解绑在线态

## Testing Strategy

第一版验证重点：

1. `channelActor -> WorldActor -> playerActor` 登录链路
2. 多端登录顶号与旧连接失效
3. `playerActor` 初始化、tick、flush、passivation
4. `WorldActor` 初始化、session 建立、世界广播
5. `player/world` 之间 request-response 超时与失败处理
6. `MongoDB` 脏数据追踪与刷盘正确性

## Migration Notes for `overmind`

相对当前 `overmind`，需要发生的关键变化是：

1. 取消 `gateway` 对 `portal/world` 的进程内直接 handler 调用
2. 新增独立 `player` 节点与 `playerActor`
3. `world` 从“场景服务”提升为独立 actor 节点
4. `portal` 从当前热链路中降为轻量外围服务预留
5. 用 actor 消息替代当前 handler 直调
6. 用 `MongoDB + DataManager + MemData` 替代当前纯内存仓储

## Risks and Mitigations

### Risk: `portal` 与 `antares-main` 路径不完全一致

Mitigation:

第一版明确把 `portal` 放在 actor 热链外，只保留边界，不把登录主流程重新拆成双入口。

### Risk: `1 worldId = 1 WorldActor` 过早成为瓶颈

Mitigation:

第一版先优先保证边界正确与数据归属正确，后续再从 `world` 内部拆 `region/cell actor`。

### Risk: MongoDB 聚合过大导致 flush 压力上升

Mitigation:

优先保证 `player` 和 `world` 的聚合边界清晰，再按具体热点把集合拆细，而不是一开始就过度范式化。

## Approval Baseline

这份设计以 `antares-main` 作为默认参考基线，并覆盖本轮已明确确认的决策：

- `player` 独立进程节点
- `player` 为玩家主数据真源
- `world` 独立进程节点
- `1 worldId = 1 WorldActor`
- `MongoDB` 为数据底座
- 后续未特别说明的设计细节默认按 `antares-main` 收敛
