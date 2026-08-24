# CoLnk Package Layout Refactor

## Status

Implemented.

## Goal

Replace the `internal/` tree with a Karpenter-style `pkg/` tree organized by runtime component and product capability. Split large source files by responsibility without changing runtime behavior.

Reference: [karpenter-provider-gcp package layout](https://github.com/cloudpilot-ai/karpenter-provider-gcp/tree/main/pkg).

## Constraints

- Keep the `colnk` and `colnk-server` runtime boundaries unchanged.
- Keep DNS names, FUSE names, and the wire protocol unchanged.
- Do not add plugin registries, dependency injection frameworks, or interfaces with only one implementation.
- Do not add runtime layers to request, filesystem, or network hot paths.
- Keep tests beside the package they cover.

## Target Layout

```text
cmd/
  colnk/
    main.go
  colnk-server/
    main.go

pkg/
  client/
    config.go
    config_test.go
    run.go

  configfile/
    load.go
    load_test.go
    paths.go
    security_unix.go
    security_other.go

  configvalidate/
    address.go
    address_test.go

  server/
    config.go
    config_test.go
    run.go
    session.go
    dns.go

  protocol/
    types.go
    message.go
    frame.go
    payload.go
    directory.go
    handshake.go
    codec_test.go

  transport/
    client.go
    client_test.go
    server.go
    conn.go

  provider/
    service.go
    audit.go
    filesystem_handler.go
    network_handler.go
    service_test.go

    filesystem/
      service.go
      path.go
      attributes.go
      directory.go
      read.go
      write.go
      mutation.go
      symlink.go
      filesystem_test.go

    network/
      policy.go
      policy_test.go

  agent/
    remote/
      remote.go
      filesystem.go
      network.go
      remote_test.go

    filesystem/
      cache.go
      cache_test.go
      mount_linux.go
      mount_stub.go
      nodes_linux.go
      symlink.go
      symlink_test.go
      write_buffer.go
      write_buffer_test.go
      write_linux.go

    network/
      network_linux.go
      network_stub.go
      firewall_linux.go
      proxy_linux.go
      system_linux.go
      dns.go
      dns_test.go
      resolver_linux.go
      resolver_stub.go
```

## Component Responsibilities

### `pkg/client`

Owns the provider command lifecycle: configuration, reconnect policy, and client sessions. It composes `transport` and `provider` but contains no filesystem or network implementation.

### `pkg/configfile`

Owns strict TOML loading, platform default paths, and platform-specific file security checks.

### `pkg/configvalidate`

Owns reusable validation for configuration values without performing file or runtime operations.

### `pkg/server`

Owns the Linux server lifecycle: configuration, listener admission, single-session ownership, FUSE/network startup, DNS startup, and shutdown ordering.

### `pkg/protocol`

Contains wire types and encoding only. It must depend only on the standard library.

### `pkg/transport`

Contains authenticated TCP and yamux session handling. It depends on `protocol` but not on client, server, provider, or agent packages.

### `pkg/provider`

Runs on the provider host and serves local resources to the remote agent. The root package owns request routing, concurrency limits, payload budgets, and auditing.

- `provider/filesystem` owns sandboxed file operations under the shared root.
- `provider/network` owns CIDR, port, and DNS policy evaluation.

### `pkg/agent`

Runs beside the cloud coding agent on Linux.

- `agent/remote` translates filesystem, TCP, and DNS calls into protocol streams.
- `agent/filesystem` exposes the remote filesystem through FUSE.
- `agent/network` creates the interface, TCP proxy, routes, DNS server, and resolver configuration.

## Current-to-Target Mapping

| Current | Target |
| --- | --- |
| `internal/clientapp` | `pkg/client` |
| `internal/serverapp` | `pkg/server` |
| `internal/protocol` | `pkg/protocol` |
| `internal/link` | `pkg/transport` |
| `internal/local` service routing | `pkg/provider` |
| `internal/local` filesystem operations | `pkg/provider/filesystem` |
| `internal/local` network policy | `pkg/provider/network` |
| `internal/agentremote` | `pkg/agent/remote` |
| `internal/agentfs` | `pkg/agent/filesystem` |
| `internal/agentnet` | `pkg/agent/network` |

## File Split Rules

- Split by lifecycle or operation, not by arbitrary line count.
- Keep one primary responsibility per file.
- Keep platform-specific code in `_linux.go`, `_darwin.go`, and `_stub.go` files.
- Keep shared state on the owning component type; do not introduce global registries.
- Keep protocol types grouped by wire responsibility so an encoding change has one obvious home.
- Keep small helpers with their caller unless they are shared by multiple files in the same package.

## Dependency Direction

```text
protocol
  ↑
transport
  ↑                 ↑
provider         agent/remote
  ↑                 ↑
client        agent/filesystem + agent/network
                    ↑
                  server
```

No package may import `client` or `server`. Provider and agent packages must not import each other.

## Migration Sequence

1. Move packages from `internal/` to `pkg/` and update imports without changing behavior.
2. Rename `link` to `transport`, `clientapp` to `client`, and `serverapp` to `server`.
3. Extract provider filesystem and network policy packages.
4. Split the large protocol, provider service, FUSE, and Linux network files by responsibility.
5. Update architecture documentation and run the existing Go and Docker tests.

## Compatibility

The recommended migration preserves all user-facing behavior and protocol compatibility. Only Go package paths change. The existing packages are under `internal/`, so external Go consumers cannot legally import them today.

The refactor intentionally does not retain forwarding packages under `internal/`; keeping them would defeat the requested layout and add no user compatibility value.

## Architecture and Product Review

### High-confidence risks

- Moving and splitting simultaneously can hide behavioral changes in review. Use mechanical moves first, then focused splits.
- `provider.Service` currently combines routing, concurrency control, filesystem dispatch, TCP proxying, DNS, and audit logging. Splitting state ownership carelessly could change request limits or shutdown behavior.
- The Linux FUSE and network files contain lifecycle ordering that must remain explicit after splitting.

### Over-engineering to avoid

- Do not create a generic component framework, service container, or plugin API.
- Do not define interfaces solely to separate files or packages.
- Do not create separate packages for every operation; files within a cohesive package are enough.
- Do not redesign the protocol while reorganizing source code.

### Smaller version

A smaller refactor could only move `internal/*` to `pkg/*`, but it would preserve the current oversized files and mixed `local` responsibilities. It would satisfy the directory request but not the component and function split request. The proposed layout is the smallest version that addresses both.
