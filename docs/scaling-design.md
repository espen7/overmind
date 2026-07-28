# 在线水平扩容设计：一致性哈希路由 + Epoch 写入围栏

> 对应实现版本：见 `internal/pkg/routing`、`internal/pkg/storage`、`internal/app/home`、`internal/app/gate`

## 1. 目标与非目标

### 目标
- **全球同服/单一大世界**架构下，Home 逻辑节点支持**不停服水平扩缩容**
- 玩家增长时"加机器像加内存一样平滑"：扩容只影响约 1/N 的玩家，且影响面为秒级迁移
- 任何时序交错（双主、脑裂、旧节点假死）下**数据不会被写坏**

### 非目标
- 不追求 Akka Cluster Sharding 式的全自动 Shard 再平衡与实体迁移。
  SLG 玩家状态有 MongoDB 兜底，Actor 只是 DB 状态的内存缓存，
  因此不需要"内存到内存"迁移，只需要保证"旧主落盘 → 新主加载"的时序。

## 2. 核心设计：安全性与有序性分离

| 关注点                   | 机制                           | 性质                   |
|--------------------------|--------------------------------|------------------------|
| **安全性**（数据不写坏） | Epoch 写入围栏 (Fencing Token) | 兜底，覆盖一切竞态     |
| **有序性**（不丢增量）   | 释放握手 (Release Handshake)   | 优化，覆盖正常迁移路径 |
| **路由一致性**           | 一致性哈希环（纯本地计算）     | 无中心查询，网关无状态 |

两者叠加后，扩容期间任何交错时序的最坏结果是：个别玩家丢失最近一个异步落盘周期
（≤5 秒）的操作 + 一次重连，等价于单机宕机的影响面，而非数据腐坏。

## 3. 一致性哈希环 (`internal/pkg/routing`)

### Ring（不可变环）
- 每个物理节点 150 个虚拟节点，crc32 哈希，`Pick(playerID)` 二分定位
- 环构建后只读，天然并发安全；任何进程对同一 (节点列表, key) 算出的归属**必然一致**
- 实测：4 节点负载偏差 < ±30%；3→4 节点扩容迁移比例 24.94%（理论 25%），
  且迁移的 key 只会迁往新节点（见 `ring_test.go`）

### Manager（双版本环管理器）
- 原子持有 `current` + `previous` 两版环，`Update()` 换环时当前环自动降级为上一版
- 版本号单调递增，拒绝过期/重复推送（防乱序）
- 用途分工：
  - **网关**：用 `current` 做路由，每个业务包校验归属（crc32 + 二分，开销可忽略）
  - **Home 协调器**：用 `previous` 反查旧归属，发起释放握手

### 配置与真相源

环的**运行期真相源是 MongoDB 环文档**（见 §6.5）；yaml 仅用于首次部署播种与
Mongo 不可达时的启动兜底：

```yaml
home_ring:            # 仅播种/兜底, 运行期以 Mongo 环文档为准
  version: 1
  nodes:
    - "home1@127.0.0.1"
  prev_nodes: []      # 仅在线扩容过渡期非空
database:             # gate/world 也需配置 (只读环文档; 缺省则退化为 yaml 静态环)
  uri: "mongodb://127.0.0.1:27017"
  db_name: "overmind"
```

## 4. Epoch 写入围栏 (`internal/pkg/storage`)

### 所有权抢占 `AcquireOwnership`
玩家文档增加 `epoch`（单调递增任期号）和 `owner_node` 字段。
协调器 Spawn PlayerActor **之前**执行：

```
findOneAndUpdate({_id: playerID}, {$inc: {epoch: 1}, $set: {owner_node: me}}, upsert)
```

MongoDB 单文档原子性保证：并发抢占时 epoch 严格递增，每个 Actor 持有唯一任期号。

### 围栏落盘 `flushBatch`
所有落盘（异步批量 / Terminate 同步）的 filter 从 `{_id}` 改为 `{_id, epoch: myEpoch}`：

- 文档已被新主抢占（epoch 递增）→ 旧主的迟到写入匹配不到文档，**静默作废**
- 不再 upsert（否则被拒写入会退化成插入脏文档）
- `MatchedCount < 模型数` 时告警日志，便于观察围栏拦截事件

### 派生影响
- `PlayerModel.Load`：文档可能已被 `AcquireOwnership` 预创建（仅含 `_id/epoch/owner_node`），
  故"文档存在但无 `profile` 分区"同样视为新玩家
- `PlayerModel.insertNew`：改为 `$set + upsert`，不覆盖围栏字段

## 5. 释放握手（Home Coordinator）

新归属节点的协调器在分配玩家时的完整流程：

```
收到网关分配请求 "playerID"
  1. 本地已有 Actor？ → 直接返回 PID（本节点已持所有权）
  2. 所有权自检 guard: 按当前环归属 ≠ 本节点？ → 返回 "error: wrong_owner"
     （堵住环切换过渡期持旧环的调用方误触发抢回所有权；调用方按其新环重试）
  3. prev 环存在且旧归属 ≠ 自己？
     → CallWithTimeout(旧节点 coordinator, "release:playerID", 10s)
        旧节点: 有 Actor → Call Actor "release" (8s, 同步全量落盘后回包退出)
                无 Actor → 直接确认
     → 失败/超时: 记日志继续（epoch 围栏兜底，最坏丢 ≤5s 增量）
  4. AcquireOwnership → 获得新 epoch（旧主迟到写入从此作废）
  5. SpawnRegister(playerID, epoch) → 从 DB 加载完整数据
```

协调器 HandleCall 的错误一律以 `"error: xxx"` 字符串作为结果返回，**绝不返回
非 nil error**——ergo 会把非 nil error 当作终止原因杀掉协调器本身。

时序关键点：PlayerActor 的 `HandleCall("release")` 在**回包之前**完成同步落盘
（ergo 保证 `(result, TerminateReasonNormal)` 先回包再终止），因此握手 Call 返回
即代表数据已安全在 DB 中；`released` 标记使 Terminate 不重复落盘。

### 积压消息的完整消费（邮箱 FIFO 保证）

ergo 中普通 Send 消息与 Call 请求（默认优先级）进的是同一条 `mailbox.Main`
FIFO 队列，因此 `"release"` Call 排在旧 Actor 邮箱**队尾**：Actor 单线程按序
先消费完全部积压业务消息，才执行 release 落盘——落盘的必然是包含全部积压
增量的最终状态。“清空邮箱 → 全量落盘 → 退位”的钝化链无需额外代码。

### 超时链对齐（外层必须包住内层）

```
gate → 新协调器            15s
  └ 新协调器 → 旧协调器     10s
      └ 旧协调器 → PlayerActor  8s
```

若外层先于内层超时（如旧 Actor 积压较深、清空邮箱超过外层等待），新协调器
会提前抢占 epoch，旧主随后的落盘被围栏作废——**安全但丢失已消费的增量**。
因此任何调整必须维持外层 > 内层 + 网络余量的不变式。

## 6. 网关侧迁移（ChannelActor）

- 登录与重绑定统一走 `allocateHome()`：环定位 → Call 协调器（15s 超时，包住握手链）
  → Link → bind_gate；失败时按原因给客户端可区分提示（`server_scaling_retry_later` /
  `server_not_ready` / `server_error`），其中环切换收敛中属短暂状态，客户端稍后重试即可
- **每包归属校验** `ensureHomeBinding()`：业务包转发前比对 `homePID.Node` 与当前环归属；
  不一致则先 `Unlink`（避免旧 Actor 退位时连带杀掉本连接）再重新分配
- 手动换环应急入口 `RingAdminActor`（注册名 `gate_ring_admin`，常规改环走 ringctl）：
  ```
  Call(gate_ring_admin@gate节点, "ring_update 2 home1@...,home2@...")
  ```

## 6.5 环文档与 ringctl（反中心化环管理）

不引入任何专职拓扑进程/中心节点，环的唯一真相源是 Mongo 单文档（`cluster_meta`
集合，`_id: "home_ring"`）：

```
{ _id: "home_ring", version: N, nodes: [...], prev_nodes: [...] }
```

- **改环**：只走 `ringctl` CLI 对文档做 CAS 写（version 乐观锁，天然串行化并发运维）
- **感知**：gate/home/world 三方每 3s 轮询（`storage.StartRingPoller`），version 变大
  即通过 `Manager.Apply` 热切环（上一版环以文档 prev_nodes 为准）；无推送、无启动依赖
- **播种**：节点启动时 `LoadOrSeedRing`（$setOnInsert + upsert），文档缺失时用 yaml
  原子播种，并发首启只有一个成功写入
- **降级**：Mongo 短暂不可达只影响“感知新环”，不影响正在使用的旧环路由；
  启动时 Mongo 不可达则回落 yaml 静态环并告警

`ringctl` 命令（`cmd/ringctl`）：

| 命令 | 作用 |
|------|------|
| `show` | 查看当前环文档 |
| `init <n1,n2,...>` | 首次创建环文档（通常由节点启动播种，极少用到） |
| `add <node>` | 扩容：新节点入环，旧环存入 prev_nodes |
| `remove <node>` | 缩容/摘除宕机节点：节点出环，旧环存入 prev_nodes |
| `commit` | 收敛完成后清空 prev_nodes（确认迁移期结束） |

防呆：prev_nodes 非空时拒绝新的 add/remove（必须先 commit）；拒绝移除最后一个节点；
CAS 冲突自动重读重试。

### 宕机处理哲学

- **平台拉活优先（兜 99%）**：k8s/supervisor 秒级重启宕机节点，环不动，玩家重连即恢复
- **人工摘环兜底（兜 1%）**：确认节点回不来才 `ringctl remove`，其玩家按新环重新分配
- **不做自动摘环**：误判（网络抖动 ≠ 宕机）、判权（谁有资格宣布节点死亡）、
  收益（epoch 围栏已保证数据安全，摘环只影响可用性）三理由下不值得引入判权复杂度

## 7. 扩缩容操作手册

### 扩容（1 节点 → 2 节点示例）

```
1. 准备 home2 配置: node 名/端口唯一; routes 需包含其余 home 节点
   （跨 home 释放握手需要互连）; home_ring 可保持旧值 (仅兜底, 以 Mongo 为准)
2. 启动 home2 进程（加入集群, 此时尚无玩家路由到它）
3. ringctl add home2@x.x.x.x
   → 文档 version+1, 旧环存入 prev_nodes
   → gate/home/world 在 ≤3s 内轮询热切
4. 迁移自动进行: 受影响的在线玩家（约 1/2）在下一个业务包时触发
   握手迁移，其余玩家零感知；离线玩家无需任何处理
5. 过渡期结束（建议 ≥ 玩家卸载超时时间）后: ringctl commit 清空 prev_nodes
```

### 缩容（节点退役）

缩容（节点退役）为同一流程反向，但**顺序不可颠倒**：必须先摘环、等收敛、再停机——
迁回时新归属节点要靠 prev 环算出旧归属仍是退役节点并向它发起释放握手，
若先停机则握手只能走超时兜底路径（丢失最近 ≤5s 未落盘增量）：

```
1. ringctl remove home2@x.x.x.x
   → 新环不含 home2, 旧环（含 home2）存入 prev_nodes
   → 三方 ≤3s 热切; 其上在线玩家下一个业务包触发反向握手迁回
2. 等收敛（建议 ≥ 玩家卸载超时时长）: ringctl commit 清空 prev_nodes
3. 此后才允许优雅停掉 home2 进程
   （停机自身会触发全量 Terminate 同步落盘, 兜住未迁走的离线玩家数据）
```

### 联跑验证剧本

两个方向均已端到端实证（客户端全程同一条连接无感，升级/金币/战力连续无损）：

| 剧本 | 验证路径 | 关键证据 |
|------|----------|----------|
| `scripts/run_scale_test.ps1` | 扩容：home1 → home2 | 环 v1→v3；epoch 1→2；新节点向旧节点发起握手 |
| `scripts/run_shrink_test.ps1` | 扩容+缩容：home1 → home2 → home1 | 环 v1→v5；epoch 1→2→3；反向握手靠 prev 环反查退役节点 |

## 8. 故障场景推演

| 场景                                           | 结果                                                                                     |
|------------------------------------------------|------------------------------------------------------------------------------------------|
| 旧节点宕机，握手超时                           | 新节点继续抢占；丢失旧主最近 ≤5s 未落盘增量（等价宕机）                                  |
| 旧 Actor 邮箱积压较深                          | FIFO 保证 release 前先消费完积压；超时链（15/10/8s）给足清空时间，极端超时才降级围栏兜底 |
| 旧 Actor 退位后才到达的消息                    | 邮箱作废/发送失败，仅丢瞬时指令（DB 状态完整）；发送方按 §10 契约处理                    |
| 旧节点假死，握手超时后旧 Actor 复活落盘        | epoch 围栏拦截，写入作废，仅告警                                                         |
| 两个网关环版本短暂不一致，双节点各起一个 Actor | 后抢占者持新 epoch，先抢占者写入全部作废；网关每包校验最终收敛到新环                     |
| 玩家迁移瞬间旧 Actor 被 Link 连带杀死网关连接  | 客户端重连，路由到新节点，数据已由握手/围栏保证完整                                      |
| 换环推送乱序/重复                              | Manager 版本号校验直接拒绝                                                               |
| 持旧环的网关/world 误找旧归属节点触发分配   | 协调器 guard 拒绝 ("error: wrong_owner")，调用方 ≤3s 后持新环重试，不会误抢所有权      |
| Mongo 短暂不可达                               | 轮询失败仅记日志，各方持旧环继续服务；恢复后自动追上最新版本                          |
| 并发运维同时改环                               | 文档 CAS 乐观锁只让一个成功，另一个重读重算；prev 未 commit 时拒绝叠加改环           |

## 9. 与 Akka Cluster Sharding 的对照

| Akka 能力    | 本方案                                                    |
|--------------|-----------------------------------------------------------|
| 位置透明路由 | ergo `ProcessID{Name, Node}` + 一致性哈希（网关本地计算） |
| 实体惰性创建 | Coordinator 查不到即 Spawn（已有）                        |
| Passivation  | 下线卸载定时器（已有）                                    |
| Shard 再平衡 | 换环 + 惰性握手迁移（DB 为迁移介质，无需状态搬运）        |
| 故障转移     | 平台拉活优先 + 人工 `ringctl remove` 兜底（见 §6.5）      |

## 10. 消息投递契约（发送方责任）

迁移窗口内，排在 `"release"` 之后进入旧 Actor 邮箱的消息会随终止作废；
Actor 死亡后的 Send 返回错误。这是 actor 消息 at-most-once 语义的必然，
DB 状态始终一致，丢的只是瞬时指令。据此约定发送方契约：

1. **关键交互必须用 Call**：任何对玩家状态有副作用且不可丢的跨 Actor 交互
   （未来的战斗结算、邮件投递、联盟操作等）必须用 Call 确认回包，
   禁止 fire-and-forget Send。
2. **失败时重解析重试**：Call 失败（超时/ErrProcessUnknown/ErrProcessTerminated）
   时，发送方按**当前环**重新计算归属节点，向新归属的 coordinator 重新分配后重试；
   重试带上限，超限进入业务级补偿（如离线邮件）。
3. **可丢消息才允许 Send**：广播、推送、表现层通知等丢了无损一致性的消息
   可用 Send，失败仅记日志。
4. **幂等兼容重试**：重试可能造成重复投递，关键交互的处理方应带业务幂等键
   （如邮件 ID、战报 ID 去重）。

当前代码中玩家 Actor 的唯一发送方是网关 ChannelActor（失败退化为踢线重连，
已满足契约）；本节约束的是未来接入的第三方系统。

## 11. TODO（后续渐进增强）

- [x] 环配置中心化：Mongo 环文档为唯一真相源 + ringctl + 三方轮询热切（§6.5）
- [ ] 宕机自动摘环 failover：若未来确有需要，应以“多方探活 + 环文档 CAS 提案”实现，
  复用同一套改环协议（当前决策：不做，见 §6.5 宕机处理哲学）
- [ ] 围栏拦截事件通知旧 Actor 主动自杀（当前依赖卸载定时器自然回收）
- [ ] 改环鉴权：ringctl 直连 Mongo，当前依赖 Mongo 访问控制；gate_ring_admin 依赖集群 cookie
- [ ] 转发墓碑：旧 Actor 退位后保留 5~10s 转发模式，把迟到消息转发给新归属
  （Akka rebalance buffering 等价物，第三方系统接入后再评估）
- [ ] k8s 部署时用 Service DNS 名替换静态 routes（或启用 ergo 内嵌 registrar 删掉 routes）
