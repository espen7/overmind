# Overmind

A high-performance SLG (Simulation Game) server framework written in Go, powered by [ProtoActor](https://github.com/asynkron/protoactor-go).

## Overview

Overmind is a distributed game server framework designed for massive concurrency and scalability, typical for 4X/SLG games. It leverages the Actor Model to manage game state and logic without complex locking mechanisms.

### Key Features

*   **Distributed Architecture**: Built on ProtoActor Cluster, allowing seamless scaling of game worlds.
*   **BigWorld Space Management**:
    *   **WorldActor (Space)**: Manages global map state and broadcasts.
    *   **CellActor (Chunk)**: Handles entity management (Marches, Resources) within partitioned spatial cells.
    *   **Seamless Handover**: Efficiently transfers entities between cells.
*   **High-Performance Gateway**: 
    *   Decoupled IO (Goroutine) and Logic (ChannelActor).
    *   Binary WebSocket protocol with AES encryption and CRC checksums.
*   **Message Mesh**:
    *   **EdgeLetter**: Client-server communication.
    *   **MeshLetter**: Internal service-to-service control commands.
    *   **Envelope**: Standardized message container with tracing support.

## Directory Structure

Following the Standard Go Project Layout:

```text
/
├── cmd/                # Entry points for each microservice
│   ├── gateway/        # Connection handling (WS/TCP)
│   ├── portal/         # Authentication & Entry (Login/Account)
│   ├── admin/          # GM/Admin backend
│   ├── world/          # World server (BigWorld Space Management)
│   └── home/           # Home server (Player City/Tech)
│
├── internal/           # Private application code
│   ├── cluster/        # ProtoActor Cluster configuration
│   ├── gateway/        # Gateway logic (NetIO, ChannelActor)
│   ├── game/           # Core game logic (World, Home)
│   ├── infra/          # Infrastructure adapters (DB, Redis)
│   └── kit/            # Internal shared tools (Config, Metrics)
│
├── pkg/                # Exported libraries
│   └── pb/             # Generated Protobuf code
│
├── api/                # API definitions
│   └── proto/          # Protobuf source files (.proto)
│       ├── kit/        # Envelope & Common messages
│       ├── gateway/    # Handshake & Heartbeat
│       └── game/       # Gameplay protocols
│
└── configs/            # Configuration files
```

## Getting Started

### Prerequisites

*   Go 1.22+
*   Protoc Compiler
*   Consul (for Cluster Provider)
*   Redis
*   MySQL/PostgreSQL

### Installation

1.  Clone the repository:
    ```bash
    git clone https://github.com/espen7/overmind.git
    cd overmind
    ```

2.  Install dependencies:
    ```bash
    go mod download
    ```

3.  Generate Protobufs:
    ```bash
    # Ensure you have protoc-gen-go installed
    ./scripts/proto_gen.sh
    ```

4.  Run Services (Development):
    ```bash
    go run cmd/world/main.go
    go run cmd/gateway/main.go
    ```

## Architecture Details

### Gateway Design
The Gateway uses a **1 Goroutine + 1 Actor** per connection model:
*   **Goroutine**: Handles blocking `ReadMessage`, CRC validation, and AES decryption.
*   **ChannelActor**: Manages session state, heartbeats, and packet routing via ProtoActor.

### Communication Pattern
*   All inter-service communication is wrapped in an **Envelope**.
*   **EdgeLetter**: Carries client payloads (`MsgType` + `Body`).
*   **MeshLetter**: Carries system commands (`Reload`, `Shutdown`).

## License

MIT
