# Contributing

## Development

Use Go 1.25 or later. Keep code, identifiers, comments, commits, and pull-request text in English.

```bash
go test ./...
make test-linux
```

Run `./scripts/test-docker.sh` for changes to FUSE, networking, authentication, TCP transport, or container permissions.

## Commits and pull requests

- Sign off every commit with `git commit -s` using a `@cloudpilot.ai` work email.
- Keep each pull request to exactly one commit.
- Open ready-for-review pull requests, not drafts.
- Never include production credentials, business metrics, customer information, or live-dashboard values in commits, issues, tests, logs, screenshots, or pull-request text. Use clearly synthetic examples.

Security reports must follow [SECURITY.md](SECURITY.md).
