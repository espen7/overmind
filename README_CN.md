# Overmind

基于 Go 语言的高性能 SLG (仿真策略) 游戏服务器框架，由 [ProtoActor](https://github.com/asynkron/protoactor-go) 驱动。

## 概览

Overmind 是一个专为大规模并发和高可扩展性设计的分布式游戏服务器框架，典型的应用场景为 4X/SLG 策略游戏。它利用 Actor 模型来管理游戏状态和逻辑，避免了传统多线程开发中复杂的锁机制。

### 核心特性

*   **分布式架构**: 基于 ProtoActor Cluster 构建，支持游戏世界无缝水平扩展。
*   **BigWorld 空间管理**:
    *   **WorldActor (Space)**: 代表一个完整的游戏地图实例，管理全局状态和广播。
    *   **CellActor (Chunk)**: 处理空间分片 (Cell) 内的实体管理（如行军部队、资源点）。
    *   **无缝切换 (Seamless Handover)**: 高效处理实体在不同 Cell 之间的所有权转移。
*   **高性能网关**: 
    *   采用 IO (Goroutine) 与 逻辑 (ChannelActor) 分离的设计。
    *   使用二进制 WebSocket 协议，内置 AES 加密和 CRC 校验。
*   **消息网格 (Message Mesh)**:
    *   **EdgeLetter**: 处理客户端与服务器之间的业务通信。
    *   **MeshLetter**: 处理内部服务之间的控制指令（如停服、重载配置）。
    *   **Envelope**: 标准化的消息容器，支持全链路追踪 (Tracing)。

## 目录结构

遵循标准 Go 项目布局 (Standard Go Project Layout):

```text
/
├── cmd/                # 微服务入口
│   ├── gateway/        # 网关服务 (处理 WS/TCP 连接)
│   ├── portal/         # 门户服务 (认证、入口、账号管理)
│   ├── admin/          # 管理后台/GM 后端
│   ├── world/          # 世界服 (BigWorld 空间管理)
│   └── home/           # 家园服 (玩家内城、科技、背包)
│
├── internal/           # 私有应用代码
│   ├── cluster/        # ProtoActor 集群配置
│   ├── gateway/        # 网关逻辑 (NetIO, ChannelActor)
│   ├── game/           # 核心游戏逻辑 (World, Home)
│   ├── infra/          # 基础设施适配器 (DB, Redis)
│   └── kit/            # 内部通用工具 (配置, 监控)
│
├── pkg/                # 可导出的公共库
│   └── pb/             # 生成的 Protobuf 代码
│
├── api/                # API 定义
│   └── proto/          # Protobuf 源文件 (.proto)
│       ├── kit/        # Envelope 信封 & 通用消息
│       ├── gateway/    # 握手 & 心跳协议
│       └── game/       # 游戏玩法协议
│
└── configs/            # 配置文件
```

## 快速开始

### 前置要求

*   Go 1.22+
*   Protoc Compiler (Protobuf 编译器)
*   Consul (用于集群服务发现)
*   Redis
*   MySQL/PostgreSQL

### 安装步骤

1.  克隆仓库:
    ```bash
    git clone https://github.com/espen7/overmind.git
    cd overmind
    ```

2.  下载依赖:
    ```bash
    go mod download
    ```

3.  生成 Protobuf 代码:
    ```bash
    # 确保已安装 protoc-gen-go
    ./scripts/proto_gen.sh
    ```

4.  运行服务 (开发模式):
    ```bash
    go run cmd/world/main.go
    go run cmd/gateway/main.go
    ```

## 架构详情

### 网关设计 (Gateway Design)
网关采用 **每连接 1 Goroutine + 1 Actor** 的模型：
*   **Goroutine**: 负责阻塞式的 `ReadMessage`，以及 CRC 校验和 AES 解密。
*   **ChannelActor**: 负责维护会话状态 (Session State)、处理心跳，并通过 ProtoActor 将消息路由到后端集群。

### 通信模式 (Communication Pattern)
*   所有服务间通信都封装在 **Envelope** (信封) 中。
*   **EdgeLetter**: 承载客户端业务数据 (`MsgType` + `Body`)。
*   **MeshLetter**: 承载系统控制命令 (`Reload`, `Shutdown`)。

## 许可证

MIT
