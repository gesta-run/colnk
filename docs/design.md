# CoLnk MVP Architecture

## 1. Goal

CoLnk lets a cloud coding agent use a Mac filesystem and controlled local TCP network access.

The MVP contains two programs:

- `colnk` runs on macOS and exposes local file and network capabilities.
- `colnk-server` runs beside the coding agent in a Linux VM, container, or Pod and creates the FUSE mount, virtual interface, and split DNS.

This is an intentionally incompatible redesign. It does not retain the old `colnk mount` command, HTTPS control plane, QUIC relay, or three-party pairing protocol.

## 2. Topology

```text
Mac
colnk start
        |
        | TCP + shared API key
        v
Agent VM / container / Pod
colnk-server
        |-- /mnt/local (FUSE)
        |-- local0 + routes
        |-- split DNS
        `-- Coding Agent
```

`colnk-server` and the coding agent must share the same filesystem and network namespace.

## 3. Command Line

macOS client:

```bash
COLNK_API_KEY=sk-xxx colnk start \
  --endpoint agent.example.com:7443 \
  --allow-cidrs 100.64.0.1/32,192.168.1.0/24,10.20.0.0/16 \
  --dns-suffixes corp.example
```

Linux server:

```bash
sudo COLNK_API_KEY=sk-xxx colnk-server \
  --listen :7443 \
  --mountpoint /mnt/local \
  --interface local0
```

The client uses `--api-key`, `COLNK_API_KEY`, then Keychain. The server uses `--api-key`, `--api-key-file`, then `COLNK_API_KEY`.

## 4. MVP Scope

Supported:

- One active Mac session per server.
- Read-write sharing of the selected Mac root, defaulting to `/`.
- FUSE file operations, directories, rename, remove, symlinks, and basic attributes.
- IPv4 TCP transparent proxying and split DNS.
- Deterministic disconnect cleanup and exponential-backoff reconnects.
- Bounded file payloads, write buffering, logical streams, and TCP connections.
- Protocol v4 directory streaming with bounded pages.

Not supported:

- Multiple users, VMs, or routed sessions.
- UDP, ICMP, layer-2 bridging, or a complete VPN.
- HTTPS, TLS, QUIC, or end-to-end encryption.
- Session tickets, signing keys, or an admin HTTP API.
- A separate mount process.
- Sharing mounts or networking across isolated namespaces.

## 5. TCP Protocol

The provider sends a length-prefixed JSON handshake containing the protocol version, API key, and requested network policy. The policy contains shared IPv4 CIDRs, optional TCP ports, and split-DNS suffixes. An empty port list allows all TCP ports in the shared CIDRs. The server applies its connection limit and returns the effective policy. Handshakes have a short timeout, and logs never include the complete API key.

Protocol v4.1 carries the provider policy. A v4.0 client receives the server's default policy; a v4.1 client rejects an older server when that server cannot honor the requested policy.

After authentication, both sides create a yamux session over the same TCP connection:

- The server opens streams for file, DNS, and TCP requests.
- The Mac accepts streams and performs local operations.
- Each file RPC uses a separate stream.
- Each proxied TCP connection uses a long-lived stream.
- Directory listings return one or more bounded response frames.

## 6. Server Lifecycle

1. Validate Linux, root privileges, FUSE, and network settings.
2. Listen for TCP connections.
3. Authenticate one provider connection and validate its requested network policy.
4. Create the yamux session.
5. Mount FUSE at `/mnt/local`.
6. Create `local0`, routes, transparent TCP rules, and split DNS.
7. Serve until disconnect or shutdown.
8. Release session admission and clean DNS, network, FUSE, and connection resources.
9. Return to listening state for reconnects.

A second Mac receives `session already active` without disrupting the current session.

## 7. Source Layout

```text
cmd/colnk/          macOS CLI
cmd/colnk-server/   Linux server CLI
pkg/client/               Client configuration, consent, and reconnect logic
pkg/server/               Listener, authentication, mount, and lifecycle
pkg/agent/remote/         Server-side file, DNS, and TCP RPC client
pkg/agent/filesystem/     FUSE implementation
pkg/agent/network/        Virtual interface, routes, proxy, and DNS
pkg/transport/            TCP handshake and yamux adapter
pkg/provider/             Mac file and network services
pkg/protocol/             RPC framing and network policy
```

## 8. Docker Simulation

Compose starts one `server` service with `/dev/fuse`, `SYS_ADMIN`, and `NET_ADMIN`. It publishes one TCP port and creates a non-root `agent` user for mount testing. It does not use host networking, privileged mode, a bind mount for `/mnt/local`, TLS material, tickets, or a separate agent service.

## 9. Performance Boundaries

- A single TCP connection can experience head-of-line blocking.
- Directory metadata is cached so child lookup and attribute requests reuse the parent listing.
- Metadata and negative entries default to a 10-second TTL and are invalidated by mutations.
- Linux FUSE enables writeback caching, asynchronous reads, and 1 MiB readahead.
- The server maintains a bounded 1 MiB block read cache with single-flight misses.
- Small, overlapping writes are merged into aligned blocks with a global memory limit; writes bypass buffering when that limit is exhausted.
- `mtime` and `fsync` may be attached to the final write, but the Mac must complete persistence before responding.
- Payloads of at least 4 KiB use fast DEFLATE only when it saves enough bytes.
- Logical streams, file operations, DNS requests, and TCP connections have separate limits.

## 10. Security Boundary

The shared API key and all data use plain TCP. This MVP is limited to Docker, local networks, controlled VPCs, or another trusted transport. It must not be presented as secure for direct public Internet exposure.

The server combines public listening, FUSE, and network administration privileges. It therefore limits handshakes, message sizes, streams, connections, and memory; avoids logging credentials, file contents, or network payloads; and performs bounded cleanup after disconnects.

## 11. Incompatible Migration

The redesign removes `colnk mount`, agent keys, VM IDs, HTTPS endpoints, TLS/CA files, session-signing keys, the QUIC UDP port, and the separate agent container. `--endpoint` now accepts `host:port`, for example `agent.example.com:7443`.

## 12. Test Priorities

- Reject invalid API keys and protocol versions.
- Reject a second active connection.
- Clean FUSE, routes, iptables, and DNS after disconnects or crashes.
- Restore mounts and networking after reconnects.
- Validate directory streaming, large-file chunking, concurrent metadata requests, and resource limits.
- Confirm that a non-root coding agent can use `/mnt/local` in Docker.

## 13. Product Review

Known high-confidence constraints are plain TCP, a shared namespace requirement, single-connection head-of-line blocking, and a privileged server whose failure affects both filesystem and networking.

The smallest complete version remains two programs, one TCP port, one shared key, and one active session. Tickets, separate pairing roles, an HTTPS control plane, QUIC, multitenancy, connection pools, and transport plugins remain out of scope.
