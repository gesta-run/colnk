# Security Policy

## Supported versions

Only the latest commit on the default branch is supported during the MVP phase.

## MVP trust boundary

- The Mac user and the Linux coding-agent environment are trusted participants.
- `colnk-server` must run in the same filesystem and network namespace as the coding agent.
- The shared API key and all payloads travel over raw TCP and are not confidential against the network.
- Use the MVP only with Docker, a trusted local network, a controlled VPC, or another trusted transport.
- Do not expose the MVP directly to the public internet.
- The default `/` read-write share is intentionally high privilege and remains limited by the Mac user's permissions and TCC grants.
- Client and server TOML files contain the shared API key. Keep them owned by the service user with mode `0600` or stricter; CoLnk rejects broader Unix permissions.
- Resource paths and network targets are omitted from logs unless resource auditing is explicitly enabled.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub private vulnerability reporting when enabled, or contact the maintainers privately with reproduction steps, affected versions, and impact.
