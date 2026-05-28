# Overmind Rewrite Design

## Context

The current `overmind` repository already has entrypoints for `gateway`, `portal`, `world`, `home`, and `admin`, but only `gateway` and `portal` contain meaningful startup code and the game-side runtime is still skeletal. The rewrite will ignore the current service design and rebuild the project around the core ideas proven by `mengfangtan/mmo-game-server`: clear service boundaries, domain-oriented packages, AOI-driven scene simulation, and a basic combat loop.

The user explicitly asked for a full rewrite on a new branch and delegated design decisions to the recommended approach. Monitoring and operations features are out of scope for this phase.

## Goals

1. Replace the current ProtoActor-centric design with a simpler service-oriented architecture.
2. Deliver a runnable first phase with three core services:
   - `gateway`: WebSocket connection management and client message routing
   - `portal`: login, token/session validation, player identity summary
   - `world`: scene runtime, AOI broadcasting, and basic combat
3. Provide a protocol and package layout that can grow into more MMO subsystems later.
4. Keep the first phase lightweight enough to compile and run locally without external ops infrastructure.

## Non-Goals

- Monitoring, dashboards, alerting, tracing backends, and other ops-focused systems
- Cross-server communication, mail, guild, chat, and social systems
- Home/city gameplay, admin backend, GM tools, and payment/account platform concerns
- Full persistence and production-grade infrastructure rollout
- Reproducing the reference repository's exact framework choices or deployment layout

## Recommended Approach

### Option A: Compatibility-first refactor

Keep the current service names and ProtoActor-oriented flow, then layer `domain/service/repository` on top.

Why not now:
- Leaves too much historical naming and runtime complexity in place
- Works against the user's intent to "fully rewrite"
- Makes later simplification harder

### Option B: Lightweight full rewrite

Rebuild the runtime around `gateway`, `portal`, and `world`, using simple Go services and domain modules instead of trying to keep ProtoActor as the center of the system.

Why this is recommended:
- Closest to the reference project's successful boundaries without importing its entire stack
- Lets us ship the core gameplay loop first
- Keeps room to reintroduce clustering or richer infrastructure later if needed

### Option C: Full stack parity with the reference project

Mirror the reference project's broad stack, including infrastructure surfaces for MQ, cache, DB, and service discovery.

Why not now:
- Too much setup cost before core gameplay works
- Pulls monitoring/ops concerns back into scope indirectly
- Increases implementation and verification time dramatically

## Target Architecture

### Service layout

`cmd/gateway`
- Accept WebSocket clients
- Decode and encode protobuf messages
- Manage connection lifecycle, heartbeats, and session binding
- Forward authenticated gameplay requests to `portal` or `world`

`cmd/portal`
- Handle login and reconnect validation
- Issue and verify session tokens
- Return lightweight player profile and spawn metadata

`cmd/world`
- Own scene state, AOI membership, and entity simulation
- Process move, enter-scene, and attack commands
- Broadcast scene events back through gateway-facing responses

Future services such as `home` or `admin` are intentionally removed from phase one instead of left as empty shells.

### Package layout

`api/proto`
- Source protobuf definitions for portal and world messages

`pkg/pb`
- Generated protobuf code

`internal/platform`
- Shared config loading
- Logging
- IDs, timing helpers, and common service bootstrap utilities

`internal/gateway`
- `app`: service wiring and startup
- `transport`: websocket server, codec, session registry
- `service`: gateway routing and connection-facing use cases

`internal/portal`
- `domain`: account, token, player summary models
- `service`: login and validation use cases
- `repository`: phase-one in-memory account/player store
- `transport`: handlers callable from gateway

`internal/world`
- `domain`: player, monster, scene, position, combat stats
- `service`: scene manager, AOI service, combat service
- `repository`: phase-one in-memory player and world state support
- `transport`: handlers callable from gateway

`pkg/aoi`
- A standalone AOI implementation that can be tested independently

This structure intentionally borrows the reference repository's domain/service/repository shape while staying idiomatic to this repo.

## Runtime and Data Flow

1. The client connects to `gateway` over WebSocket.
2. `gateway` performs handshake and heartbeat tracking.
3. Login requests are routed to `portal`.
4. `portal` validates credentials, returns a session token, player ID, and default scene entry data.
5. `gateway` binds the session to the authenticated player.
6. Enter-scene and gameplay messages are routed to `world`.
7. `world` loads or creates the player entity, places it into a scene, updates AOI visibility, and emits enter/leave/snapshot events.
8. Move and attack commands mutate scene state and produce broadcast events for nearby players.
9. `gateway` encodes outbound events and pushes them to subscribed client sessions.

For phase one, service-to-service calls can remain in-process package calls or thin local facades as long as the boundaries stay explicit. The code should be structured so RPC or message bus adapters can be introduced later without rewriting domain logic.

## Gameplay Scope for Phase One

### Included

- Account login with a simple credential source suitable for local development
- Session token issuance and validation
- Scene entry and scene snapshot
- AOI visibility updates for player movement
- Basic monster entities in a scene
- Basic combat command flow:
  - player attacks monster
  - damage calculation
  - HP updates
  - death event emission
- Broadcast of nearby movement and combat results

### Excluded

- Skill trees, buff systems, loot tables, inventories, equipment, quests
- Sophisticated AI behavior beyond minimal targetable monsters
- Matchmaking, instances, cross-scene travel, and cross-server features
- Persistent player progression beyond what is needed for local flow

## Error Handling

- Transport errors stay in `gateway` and result in clean session teardown
- Portal and gameplay failures return explicit protobuf error responses with stable error codes
- Domain services return typed errors instead of leaking transport or storage details
- Invalid scene or combat commands are rejected without crashing the scene loop
- Logs should capture connection ID, player ID, message type, and error code where available

## Testing Strategy

### Unit tests

- AOI grid membership and neighbor visibility
- Damage calculation and combat outcome rules
- Session token generation and validation
- Message codec round-trip behavior

### Service-level verification

- `gateway`, `portal`, and `world` compile independently
- A smoke flow verifies login, scene entry, movement, and one attack round

The first implementation pass should prefer deterministic tests around AOI and combat because those are the most behavior-dense parts of the rewrite.

## Delivery Plan

1. Create a new branch for the rewrite.
2. Remove or replace the old `portal/world/home/admin` skeletons.
3. Introduce the new package layout and shared bootstrap code.
4. Define protobuf messages for portal and scene/combat flow.
5. Implement gateway transport and session binding.
6. Implement portal service with in-memory repository.
7. Implement world scene manager, AOI, and combat service.
8. Add tests for AOI, portal token flow, and combat calculation.
9. Run build and smoke verification.

## Risks and Mitigations

### Risk: Over-copying the reference repository

Mitigation:
Use the reference project for boundaries and feature priorities, not as a mandate to reproduce every dependency or subsystem.

### Risk: Hidden coupling from the old codebase

Mitigation:
Treat existing packages as disposable unless they are clearly reusable helpers such as config or logging.

### Risk: Scope growth

Mitigation:
Lock phase one to login, scene entry, AOI, and basic combat. Defer everything else.

## Approval and Assumptions

This spec uses the recommended option because the user explicitly authorized proceeding with recommended decisions while away. Unless contradicted later, this document is the approved design baseline for implementation.
