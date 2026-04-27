# youtrack-mcp

Standalone MCP server for YouTrack, implemented in Go.

## Features

- exposes a compact YouTrack-only MCP toolset: `fetch`, `get_statuses`, `update_status`, `create`, `update`, `link`, `unlink`
- runs as a local stdio MCP server for IDE or CLI integrations
- registers itself into VS Code or Copilot MCP config, globally or per workspace
- reads YouTrack credentials from environment variables, a local `.env`, or `--env-file`

## Configuration

Set these environment variables directly, put them in a `.env` file in the working directory, or pass `--env-file`:

```bash
YOUTRACK_URL=https://your-youtrack-instance.example
YOUTRACK_API_TOKEN=perm:your-token
YOUTRACK_HTTP_TIMEOUT=45s
```

Existing process environment variables take precedence over values loaded from `.env` or `--env-file` unless you pass `--override-env`.
`YOUTRACK_HTTP_TIMEOUT` is optional and uses Go duration syntax such as `15s`, `45s`, or `2m`.

## Usage

### Start the MCP server

```bash
go run ./cmd/youtrack-mcp mcp
```

With an explicit env file:

```bash
go run ./cmd/youtrack-mcp --env-file /path/to/.env mcp
```

Force env-file values to override the current process environment:

```bash
go run ./cmd/youtrack-mcp --env-file /path/to/.env --override-env mcp
```

### Register the server

Register globally for VS Code:

```bash
go run ./cmd/youtrack-mcp install vscode
```

Register only for one workspace:

```bash
go run ./cmd/youtrack-mcp install vscode --workspace /path/to/project
```

Register for Copilot:

```bash
go run ./cmd/youtrack-mcp install copilot
```

Workspace-local Copilot registration:

```bash
go run ./cmd/youtrack-mcp install copilot --workspace /path/to/project
```

If you register from a development checkout, prefer a built binary or pass `--binary /path/to/youtrack-mcp` so the stored command path stays stable.

### Exposed MCP tools

| Tool | Purpose |
|---|---|
| `search` | Search tickets by free text or YouTrack query syntax and return structured issue data |
| `fetch` | Fetch a ticket as structured JSON by default, or Markdown when `markdown=true`; attachments can be inlined or saved in either mode |
| `get_statuses` | List valid statuses for a ticket |
| `update_status` | Change a ticket status |
| `create` | Create a ticket |
| `update` | Update summary, description, or custom fields |
| `link` | Link one ticket to another |
| `unlink` | Remove a ticket link |

All ticket-scoped tools require an explicit `ticket`. The MCP server always uses the configured `YOUTRACK_URL` and `YOUTRACK_API_TOKEN`; MCP calls cannot override them.

## Common workflow

1. Build the binary.
2. Configure `YOUTRACK_URL` and `YOUTRACK_API_TOKEN`.
3. Run `install vscode` or `install copilot`.
4. Start a new IDE or Copilot session so the MCP config is picked up.
5. Use the compact tool names from your MCP-enabled client.

## Building

```bash
go build ./cmd/youtrack-mcp
```

For development details, architecture, and release/build notes, see [`DEV.md`](./DEV.md).
