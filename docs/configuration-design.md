# Configuration File Design

## Status

Proposed for review. Implementation must follow this document after the compatibility decision is confirmed.

## Goals

- Configure both programs from TOML files, including endpoints, API keys, ports, filesystem sharing, networking, DNS, logging, and runtime limits.
- Keep normal startup free of business configuration flags and environment variables.
- Make unattended startup predictable for local development, system services, containers, and future platforms.
- Reject ambiguous, unknown, or unsafe configuration instead of silently ignoring it.

## Commands and Paths

Normal startup reads one deterministic default file:

```text
colnk
colnk-server
```

Only bootstrap flags remain:

```text
colnk --config /path/to/client.toml
colnk-server --config /path/to/server.toml
colnk --help
colnk --version
```

Default paths:

| Program | Platform | Path |
| --- | --- | --- |
| `colnk` | macOS | `~/Library/Application Support/CoLnk/client.toml` |
| `colnk` | Linux | `${XDG_CONFIG_HOME:-~/.config}/colnk/client.toml` |
| `colnk-server` | Linux | `/etc/colnk/server.toml` |

The current directory is never searched implicitly. This prevents a repository from supplying configuration or credentials merely because the command was launched inside it.

## Client Configuration

```toml
endpoint = "agent.example.com:7443"
root = "/"

[auth]
apiKey = "sk-example"

[network]
allowCIDRs = [
  "100.64.0.1/32",
  "192.168.1.0/24",
  "10.20.0.0/16",
]

# An empty list allows every TCP port in allowCIDRs.
allowPorts = []
dnsSuffixes = ["colnk", "corp.example"]

[logging]
auditResources = false
```

Defaults:

| Key | Default |
| --- | --- |
| `root` | `/` |
| `network.allowCIDRs` | `["100.64.0.1/32"]` |
| `network.allowPorts` | `[]`, meaning all TCP ports |
| `network.dnsSuffixes` | `["colnk"]` |
| `logging.auditResources` | `false` |

`endpoint` and `auth.apiKey` are required. The configuration file itself is explicit consent for the listed filesystem and network shares, so the interactive risk prompt is removed.

## Server Configuration

```toml
listen = ":7443"
mountpoint = "/mnt/local"
allowOther = true
metadataCacheTTL = "10s"

[auth]
apiKey = "sk-example"

[network]
interface = "local0"
proxyPort = 15001
maxTCPConnections = 256

[dns]
listen = "127.0.0.1:53"
upstream = "1.1.1.1:53"
configureResolver = true
```

Defaults remain equivalent to the current server defaults. `auth.apiKey` is required. Provider CIDRs, ports, and DNS suffixes are not server configuration; they arrive in the authenticated client handshake.

## Loading and Validation

1. Resolve the explicit path or the single platform default.
2. Open the file without following an unsafe ownership or permission state.
3. Decode TOML with unknown-field rejection.
4. Apply documented defaults.
5. Validate the complete configuration before opening sockets, mounting FUSE, or changing routes and DNS.
6. Redact `auth.apiKey` from errors and logs.

Validation includes:

- endpoint and listen-address syntax;
- IPv4 CIDR syntax;
- TCP port range and duplicate removal;
- DNS suffix syntax;
- mountpoint and root normalization;
- duration and connection-limit bounds;
- bounded list sizes to prevent excessive routes or handshake payloads.

On Unix, files containing `auth.apiKey` must be owned by the running user and have no group or other permissions. Recommended modes are `0600` for editable files and `0400` for service-managed files. Server configuration is normally owned by root.

## Runtime Behavior

- Configuration is loaded once at process start.
- Invalid or missing configuration terminates startup with the path and field name, but never the API key value.
- Reconnects reuse the already validated in-memory configuration.
- Hot reload and `SIGHUP` are out of scope for the MVP. A service restart applies changes atomically.
- Example files live under `configs/` and contain only illustrative credentials and networks.

## Compatibility Decision

Recommended: make this an intentional clean break while the project has no released compatibility contract.

- Remove `colnk start` and all business configuration flags.
- Remove API-key environment and Keychain precedence.
- Remove business flags from `colnk-server`.
- Keep only `--config`, `--help`, and `--version`.
- Keep protocol v4.1 unchanged because the on-wire network policy already has the correct ownership.

Alternative: retain the old command and flags for one release, print deprecation warnings, and make TOML take precedence. This reduces migration friction but creates two configuration paths and substantially more test surface.

## Implementation Boundaries

- Add a small shared configuration-file package for path resolution, TOML decoding, and Unix permission checks.
- Keep client and server schema/default validation in their existing packages.
- Add example client and server TOML files.
- Update installers, Docker Compose, tests, README, security documentation, and architecture documentation together.
- Do not add automatic route discovery or hot reload as part of this change.
