# Development guide

## Build

Format the code:

```bash
go fmt ./...
```

Run tests:

```bash
go test ./...
```

Run a single test:

```bash
go test ./internal/youtrack -run TestClientOperations
```

Build the local binary:

```bash
go build ./cmd/youtrack-mcp
```

Build the release binary into `dist/`:

```bash
make build
```

Cross-build for supported targets:

```bash
make cross-build
```

Current targets:

- `darwin/amd64`
- `darwin/arm64`
- `linux/amd64`
- `linux/arm64`
- `windows/amd64`
- `windows/arm64`

All builds use `CGO_ENABLED=0`.

The module targets Go 1.22+.

## Testing

The project currently uses standard Go tests:

- `internal/youtrack/client_test.go` exercises the YouTrack client against a local fake HTTP server
- `internal/install/registration_test.go` covers MCP config generation and JSONC upserts
- `internal/envfile/envfile_test.go` covers dotenv parsing
- `internal/mcp/server_test.go`, `internal/cli/root_test.go`, and `internal/config/config_test.go` cover MCP wiring, CLI env loading, and timeout parsing

There is no separate lint step today beyond `go fmt`.

## Deploying

This project is deployed as a local binary, not as a hosted service.

GitHub Actions runs the Go test suite on pushes and pull requests, and runs `make cross-build` on version tags to validate release artifacts.

Typical release flow:

1. Run `go test ./...`
2. Run `make cross-build`
3. Distribute the binary for the target platform from `dist/`
4. On the target machine, configure `YOUTRACK_URL` and `YOUTRACK_API_TOKEN`
5. Run `youtrack-mcp install vscode` or `youtrack-mcp install copilot`

For local development installs, prefer:

```bash
./youtrack-mcp install vscode --binary "$(pwd)/youtrack-mcp"
```

That avoids registering a temporary `go run` binary path.

## Internal architecture

### CLI layer

- `cmd/youtrack-mcp/main.go` is the executable entrypoint
- `internal/cli` contains Cobra command definitions
- `root.go` applies `.env` or `--env-file` before running any subcommand

Top-level commands:

- `mcp` starts the stdio MCP server
- `install` writes MCP config for VS Code or Copilot

### MCP layer

`internal/mcp/server.go` creates the MCP server with the Go MCP SDK and registers the public tools.

The MCP surface defaults to a `youtrack_` prefix so it does not collide with tools from other MCP servers. Set `YOUTRACK_TOOL_PREFIX=""` to opt out when the client already scopes tools by server:

- `youtrack_search`
- `youtrack_fetch`
- `youtrack_fetch_markdown`
- `youtrack_get_statuses`
- `youtrack_update_status`
- `youtrack_create`
- `youtrack_update`
- `youtrack_link`
- `youtrack_unlink`

The handler layer is thin by design: it resolves credentials, validates required ticket IDs for ticket-scoped tools, then delegates to the domain client.

### YouTrack client

`internal/youtrack` is split into focused files instead of one monolithic client file:

- `client.go` for client construction and URL validation
- `issues.go` for ticket fetch/search/create/update flows
- `commands.go` for statuses, links, and YouTrack command execution
- `attachments.go` for attachment download/storage and markdown rendering helpers
- `http.go` for shared request/retry/body-limit handling
- `serialize.go` and `types.go` for serialization, helpers, and shared types/constants

This package is the main domain layer of the application.

### Install and registration

`internal/install/registration.go` owns MCP registration behavior.

It handles:

- target-specific config shape (`servers` for VS Code, `mcpServers` for Copilot)
- global vs workspace-local config paths
- safe upsert of this server into an existing config
- HuJSON-based updates so existing JSONC comments survive config edits
- stable binary resolution, with an explicit guard against registering a temporary `go run` binary

### Supporting packages

- `internal/envfile` loads dotenv-style files
- `internal/config` holds env var names and constants
- `YOUTRACK_HTTP_TIMEOUT` can override the default 30s HTTP client timeout with a Go duration string
- `YOUTRACK_TOOL_PREFIX` defaults tool names to the `youtrack_` prefix; set it to an empty string to expose unprefixed names
- `.env` support is intentionally single-line and minimal: matching outer quotes are stripped, but escape sequences are not processed
- `internal/version` is populated at build time from `git describe --tags --always --dirty` and exposed by the CLI and MCP server metadata

## Design decisions

### Short MCP tool names

The tool names are intentionally generic and compact because the entire server is already scoped to YouTrack. Repeating `youtrack_` in every tool name added noise without adding meaning.

### Thin MCP handlers

The MCP layer is kept small so protocol concerns stay separate from YouTrack business logic. This makes the YouTrack client easier to test independently and keeps the public MCP surface easy to audit.

### Configured credentials only

The MCP layer does not accept per-tool URL or token overrides. The server always uses the configured `YOUTRACK_URL` and `YOUTRACK_API_TOKEN`, loaded from the process environment or from `.env` / `--env-file`. Values loaded from `.env` do not overwrite environment variables that are already set in the process unless the CLI is started with `--override-env`.

### Config upsert instead of overwrite

Installer commands only update this server entry in the target MCP config. They do not rewrite unrelated configuration, which is safer on developer machines that already use other MCP servers.
