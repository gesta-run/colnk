# CoLnk MVP Test Cases

## 1. Release Gate

- `go test ./...` passes.
- `make test-linux` passes.
- `docker compose config --quiet` passes.
- `./scripts/test-docker.sh` passes.
- Logs contain neither complete API keys nor file contents.

## 2. TCP Handshake

### Successful connection

- Start the server and provider client with the same API key.
- Expect authentication to succeed and the server to create FUSE, `local0`, and split DNS.

### Invalid API key

- Connect with the wrong key.
- Expect a permanent rejection, no mount or network resources, and no key in logs.

### Duplicate connection

- Keep one provider session active and start a second client.
- Expect `session already active`; the first session remains unaffected.

### Disconnect and reconnect

- Establish a session, restart the server, and wait for automatic reconnect.
- Expect old mounts and network rules to be removed and recreated without duplicate routes or iptables rules.

## 3. FUSE

- List the shared provider root from the server environment.
- Create, write, read, rename, and remove files.
- Create absolute and relative symlinks and verify target mapping.
- Modify a previously read file on the provider and read it again from the server.
- Transfer a large file in 1 MiB chunks and compare hashes.
- Read a 16 MiB file sequentially in 128 KiB requests and confirm no more than 16 provider-side read RPCs.
- Read the same uncached block concurrently and confirm only one miss RPC.
- Perform out-of-order and overlapping writes to one block and compare hashes.
- Call `fsync` and confirm the final write completes data, modification time, and persistence without a separate trailing RPC.
- Confirm special device files are rejected and paths cannot escape the shared root.

## 4. Directory Streaming

- List directories that require multiple protocol response pages.
- Verify every entry appears exactly once and each page remains within its payload limit.
- Confirm a listing does not issue per-child `stat` or `readlink` RPCs.
- Mutate a directory while a child read is in flight and confirm stale data is not restored to cache.

## 5. Network

- Resolve `host.colnk` to `100.64.0.1`.
- Access an allowed provider service bound to `127.0.0.1`.
- Attempt access to a denied port.
- Confirm ordinary public DNS still uses the upstream resolver.
- Attempt ICMP and direct UDP.
- Expect allowed TCP to succeed and denied TCP, ICMP, and UDP not to reach the provider.

## 6. Permissions and Isolation

- The server container does not use host networking or `--privileged`.
- The server receives only `SYS_ADMIN`, `NET_ADMIN`, and `/dev/fuse`.
- `/mnt/local` is not a host bind mount.
- The non-root `agent` user can use the mount when `allowOther = true` is configured.

## 7. Resource Limits

- Send frames beyond the header, 1 MiB request, or 8 MiB response limits.
- Send unknown compression encodings, corrupt DEFLATE data, false raw lengths, and decompressed payloads beyond the limit.
- Exceed logical stream, file request, DNS request, and TCP connection limits.
- Open many dirty files and confirm global write memory remains bounded without deadlock.
- Connect without completing the handshake.
- Expect rejection or timeout without sustained memory or goroutine growth.

## 8. Performance Smoke Test

- Compare cold and warm `ls -la` on a directory with files, directories, and symlinks.
- Confirm repeated listings within the metadata TTL do not create file RPCs.
- Transfer compressible source text and random data; only the first should use DEFLATE, and both hashes must match.
- Write at least 16 MiB of compressible data and call `fsync`; each complete 1 MiB block should produce at most one write RPC.
- Run directory traversal, small-file operations, a large transfer, and multiple long-lived TCP connections concurrently.
- Expect completion without deadlock and record throughput and P95 latency for future comparison.

## 9. Known Non-Goals

The MVP does not validate public-Internet confidentiality, TLS, QUIC, multitenancy, multiple VMs, UDP, ICMP, layer-2 networking, or cross-namespace mount sharing.
