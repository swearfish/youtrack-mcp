# youtrack-mcp

Standalone MCP server for YouTrack, implemented in Go.

## Commands

```bash
go run ./cmd/youtrack-mcp mcp
go run ./cmd/youtrack-mcp install vscode
go run ./cmd/youtrack-mcp install vscode --workspace /path/to/project
go run ./cmd/youtrack-mcp install copilot
go run ./cmd/youtrack-mcp install copilot --workspace /path/to/project
```

If you use `install`, prefer running a built binary or pass `--binary /path/to/youtrack-mcp` so the registered command path is stable.

## Configuration

The server reads these environment variables directly or from `--env-file` / a local `.env` file:

```bash
YOUTRACK_URL=https://your-youtrack-instance.example
YOUTRACK_API_TOKEN=perm:your-token
```

## Build and test

```bash
go test ./...
go build ./cmd/youtrack-mcp
make cross-build
```
