<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="design-assets/colnk-logo/final-svg/colnk-horizontal-white.svg">
    <img src="design-assets/colnk-logo/final-svg/colnk-horizontal-transparent.svg" alt="CoLnk" width="300">
  </picture>
</p>

<p align="center"><strong>Use local filesystems and TCP services from a remote coding-agent environment.</strong></p>

<p align="center">
  <a href="https://github.com/gesta-run/colnk/actions/workflows/ci.yml"><img src="https://github.com/gesta-run/colnk/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue.svg" alt="Apache 2.0 license"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg" alt="Go 1.25+"></a>
</p>

CoLnk connects a remote agent environment to a provider host through interfaces that existing tools already understand:

- The provider filesystem appears as a standard FUSE mount.
- Approved provider-side TCP services are reachable through normal routes and split DNS.
- Coding agents, shells, editors, and build tools need no CoLnk-specific integration.

> [!WARNING]
> CoLnk is an MVP. The current transport uses a shared API key over unencrypted TCP. Run it only through Docker, a trusted private network, a controlled VPC, or another trusted transport. Do not expose it directly to the public Internet.

## How it works

![CoLnk architecture](docs/assets/how-it-works.svg)

`colnk` opens one outbound TCP connection from the provider host and carries filesystem, TCP, and DNS traffic over multiplexed streams. `colnk-server` exposes those resources to the agent environment through a FUSE mount, routes, and split DNS.

## Features

- Read-write FUSE access to `/` by default, or to a configured root.
- Transparent IPv4 TCP access controlled by CIDR and port allowlists.
- `host.colnk` for reaching approved services bound to provider loopback.
- Automatic reconnect with bounded FUSE and network cleanup.
- Streamed directory listings, block caching, write aggregation, and payload compression.
- Strict TOML configuration with explicit filesystem and network policy.
- Docker-based end-to-end development environment.

## Quick start with Docker

Requirements:

- macOS
- Go 1.25 or later
- Docker Desktop with FUSE support
- OpenSSL

Start the simulated Linux coding-agent environment:

```bash
make docker-up
```

In a second terminal, start the provider client:

```bash
./scripts/run-local-dev.sh
```

The first run creates `.colnk-dev/client.toml` and `.colnk-dev/server.toml` with a shared development key. Edit `root` in `client.toml` to narrow the default `/` share.

Open a shell as the simulated coding-agent user:

```bash
docker compose exec --user agent server sh
ls -la /mnt/local
getent hosts host.colnk
```

Stop the environment when finished:

```bash
make docker-down
```

## Install from source

### macOS client

```bash
./scripts/install-macos.sh
```

This installs `colnk` to `~/.local/bin` by default. Override the destination with `COLNK_INSTALL_DIR`.

### Linux server

Build the Linux AMD64 binary on any supported Go host:

```bash
make build-linux
```

The resulting binary is `bin/colnk-server-linux-amd64`. The target Linux environment requires:

- FUSE 3 and `/dev/fuse`
- `iproute2` and `iptables`
- root, or equivalent `SYS_ADMIN` and `NET_ADMIN` capabilities
- inbound reachability on the configured CoLnk TCP port

## Connect a provider to a Linux agent host

Generate one shared key and place it in protected TOML files on both hosts. Example files are available in [`configs/`](configs/).

On the Linux agent host:

```bash
sudo install -d -m 700 /etc/colnk
sudo install -m 600 configs/server.toml.example /etc/colnk/server.toml
sudo editor /etc/colnk/server.toml
```

Start the server in the coding agent's namespace:

```bash
sudo ./colnk-server
```

On the provider host, create the default client configuration and set the same API key:

```bash
mkdir -p "$HOME/Library/Application Support/CoLnk"
cp configs/client.toml.example "$HOME/Library/Application Support/CoLnk/client.toml"
chmod 600 "$HOME/Library/Application Support/CoLnk/client.toml"
editor "$HOME/Library/Application Support/CoLnk/client.toml"
colnk
```

Use `--config /path/to/file.toml` on either program to override the default path. Business settings are deliberately not accepted through command-line flags or environment variables.

After the session is ready, Linux tools can use the shared filesystem directly:

```bash
cd /mnt/local
git status
```

## Local network access

The default policy exposes only the virtual host address `100.64.0.1/32`. `host.colnk` resolves to that address and maps TCP connections back to provider loopback.

The provider chooses which local networks and ports to expose in `client.toml`. An empty `allowPorts` list, which is the default, allows all TCP ports in the selected CIDRs:

```toml
[network]
allowCIDRs = ["100.64.0.1/32", "192.168.1.0/24", "10.20.0.0/16"]
allowPorts = []
dnsSuffixes = ["colnk", "corp.example", "internal.example"]
```

Set `allowPorts = [22, 80, 443, 5432]` when the share should be restricted to specific TCP ports.

The Linux environment can then use ordinary clients:

```bash
curl http://host.colnk:3000
psql -h host.colnk -p 5432
```

CoLnk currently proxies IPv4 TCP only. It does not forward UDP, ICMP, Ethernet frames, or arbitrary VPN traffic.

## Security model

- The provider host and Linux agent environment are both trusted participants.
- The current macOS client must run as a non-root user.
- Filesystem access is limited by the provider process permissions and platform security controls.
- The default share is `/` with read-write access; configure `root` to reduce the exposed scope.
- Network access is limited by the provider-selected CIDRs, ports, and DNS suffixes; the server enforces the connection limit.
- File paths, DNS names, and TCP targets are omitted from logs unless `logging.auditResources` is enabled.
- One server accepts one active provider session.
- The API key and payloads are not encrypted by CoLnk in the current MVP.

See [SECURITY.md](SECURITY.md) before deploying outside a local development environment.

## Configuration highlights

| Component | TOML key | Default | Purpose |
| --- | --- | --- | --- |
| Client | `endpoint` | required | CoLnk server in `host:port` form |
| Client | `root` | `/` | Provider directory exposed through FUSE |
| Client | `network.allowCIDRs` | `100.64.0.1/32` | Reachable provider-side IPv4 ranges |
| Client | `network.allowPorts` | all allowed CIDR ports | Reachable TCP ports |
| Client | `network.dnsSuffixes` | `colnk` | Names resolved through split DNS |
| Server | `mountpoint` | `/mnt/local` | FUSE mount visible to the coding agent |
| Server | `network.maxTCPConnections` | `256` | Concurrent proxied TCP limit |
| Server | `metadataCacheTTL` | `10s` | FUSE metadata cache lifetime |

Both files require `auth.apiKey`. They must be owned by the running user and inaccessible to group and other users. See the [configuration reference](docs/configuration-design.md) for complete schemas, defaults, paths, and validation rules.

## Build and test

```bash
make build
make test
make test-linux
./scripts/test-docker.sh
```

The Docker test covers authentication, FUSE operations, transparent TCP, split DNS, namespace isolation, reconnects, and credential-safe logging.

## Current limitations

- macOS client and Linux server only.
- The server and coding agent must share the same filesystem and network namespace.
- A single TCP connection carries all yamux streams and can experience head-of-line blocking.
- Hard links, xattrs, mmap, offline writes, and complete POSIX open-handle semantics are not implemented.
- No multitenancy, multiple active providers, session routing, online revocation, or encrypted transport.

## Documentation

- [Architecture](docs/design.md)
- [Configuration](docs/configuration-design.md)
- [Test cases](docs/test-cases.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## License

Licensed under the [Apache License 2.0](LICENSE).
