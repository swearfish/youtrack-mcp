# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- Run all tests: `go test ./...`
- Run one test: `go test ./internal/youtrack -run TestClientOperations`
- Format: `go fmt ./...` (or `make fmt`)
- Build CLI: `go build ./cmd/youtrack-mcp` (or `make build` for `dist/youtrack-mcp`)
- Cross-build all release targets: `make cross-build` (darwin/linux/windows × amd64/arm64)
- Run server locally: `go run ./cmd/youtrack-mcp mcp`

## Architecture

Standalone Go binary that exposes a YouTrack-only MCP server over stdio plus an installer that registers itself with VS Code or Copilot. The Cobra entrypoint in `cmd/youtrack-mcp` wires two subcommands:

- `mcp` — starts the stdio MCP server (`internal/mcp` registers tools against `github.com/modelcontextprotocol/go-sdk`)
- `install [vscode|copilot]` — upserts an MCP server entry into the host's config file

Internal packages have narrow responsibilities:

- `internal/mcp` — declares the seven exposed tools (`fetch`, `get_statuses`, `update_status`, `create`, `update`, `link`, `unlink`) and translates MCP arg structs into `youtrack.Client` calls. The `resolveClientAndTicket` / `resolveClient` helpers centralize configured-credential and ticket validation for every tool.
- `internal/youtrack` — HTTP client for the YouTrack REST API; field selection strings (`issueDetailFields`, `defaultStoryIssueFields`, etc.) are defined as package constants.
- `internal/install` — picks the right config path per target/OS (with `VSCODE_MCP_CONFIG_PATH` / `COPILOT_MCP_CONFIG_PATH` overrides and `--workspace` for project-local configs), then upserts the entry while **preserving unrelated settings**. JSONC comments in existing configs are stripped before parsing via `stripJSONCComments`.
- `internal/envfile` — loads dotenv values either from `--env-file` or, if absent, an auto-detected `.env` in the working directory. Applied in the root command's `PersistentPreRunE` before any subcommand executes.
- `internal/config` — constants only: server name, env var names (`YOUTRACK_URL`, `YOUTRACK_API_TOKEN`), HTTP timeout.

### Tool input conventions

- Tool names stay short (single verbs/nouns) because the server is YouTrack-only; argument JSON keys are `snake_case` for MCP compatibility, even when the Go struct fields are CamelCase.
- Every ticket-scoped tool requires `ticket`. MCP calls do not accept `youtrack_url` or `youtrack_token`; the server always uses the configured environment values and errors explicitly if they are missing.

### Installer behavior worth knowing

- VS Code config uses `servers` key with `type: "stdio"`; Copilot config uses `mcpServers` key with `type: "local"` and `tools: ["*"]`. Don't conflate the two shapes.
- `resolveBinaryPath` rejects paths containing `go-build` so users don't accidentally register a `go run` temp binary; in that case they must build first or pass `--binary`.
- If `--env-file` was supplied to the root command, the installer embeds `--env-file <abs-path>` into the registered command's args so the server keeps its env on subsequent launches.

## Scope guardrails

This project is intentionally limited to YouTrack. Keep tools, packages, and CLI surface focused on that scope.
