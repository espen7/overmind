# Overmind

A lightweight MMO game server playground written in Go.

This branch rewrites the original skeleton around three core services inspired by `mengfangtan/mmo-game-server`, while intentionally excluding monitoring and operations infrastructure.

## Phase-One Scope

- `gateway`: WebSocket access, packet framing, session binding
- `portal`: login entry, token issuance, and pre-game entry context
- `game`: scene entry, AOI visibility, and basic combat

## Current Layout

```text
cmd/
  portal/    # portal service entrypoint
  game/      # game service entrypoint
  gateway/   # websocket gateway entrypoint

internal/
  portal/    # portal domain, repository, service, transport
  game/      # scene, combat, world state, transport
  gateway/   # websocket transport and binary packet framing
  platform/  # config, logging, lifecycle helpers

pkg/
  aoi/       # standalone AOI grid
  pb/        # generated protobuf code

api/proto/
  portal/    # portal protobuf contracts
  game/      # game protobuf contracts
```

## Excluded for Now

- Monitoring, metrics dashboards, tracing, and alerting
- Cross-server messaging and cluster orchestration
- Home/city gameplay, admin backend, and GM tooling
- Persistent database-backed progression

## Local Development

### Generate protobuf code

```powershell
.\scripts\proto_gen.bat
```

### Run tests

```powershell
go test ./...
```

### Start services

```powershell
go run ./cmd/portal
go run ./cmd/game
go run ./cmd/gateway
```

Health endpoints:

- `http://127.0.0.1:8081/healthz`
- `http://127.0.0.1:8082/healthz`
- `http://127.0.0.1:8080/healthz`

## Notes

The current implementation keeps service boundaries explicit but uses in-memory repositories and local process wiring so the core gameplay loop can be exercised without external infrastructure.
