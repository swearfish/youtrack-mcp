# youtrack-mcp

Standalone MCP server for YouTrack, implemented in Go.

## Features

- exposes a configurable YouTrack-only MCP toolset, defaulting to `youtrack_search`, `youtrack_fetch`, `youtrack_fetch_markdown`, `youtrack_get_statuses`, `youtrack_update_status`, `youtrack_create`, `youtrack_update`, `youtrack_link`, `youtrack_unlink`
- exports YouTrack tickets as MCP resources via `youtrack://TICKET-1234` and `youtrack://TICKET-1234/markdown` URIs
- runs as a local stdio MCP server for IDE or CLI integrations
- registers itself into VS Code or Copilot MCP config, globally or per workspace
- reads YouTrack credentials from environment variables, a local `.env`, or `--env-file`

## Configuration

Set these environment variables directly, put them in a `.env` file in the working directory, put them in a `.env` file next to the binary, or pass `--env-file`:

```bash
YOUTRACK_URL=https://your-youtrack-instance.example
YOUTRACK_API_TOKEN=perm:your-token
YOUTRACK_HTTP_TIMEOUT=45s
YOUTRACK_TOOL_PREFIX=youtrack_
```

Existing process environment variables take precedence over values loaded from `.env` or `--env-file` unless you pass `--override-env`. When `--env-file` is omitted, the CLI auto-loads `.env` from the current working directory first, then falls back to the binary's parent directory.
`YOUTRACK_HTTP_TIMEOUT` is optional and uses Go duration syntax such as `15s`, `45s`, or `2m`.
`YOUTRACK_TOOL_PREFIX` defaults to `youtrack_`. Set it to an empty value to expose unprefixed tool names like `fetch` and `update_status`, or set it to a different prefix if your client expects one.
`.env` parsing is intentionally simple: values must stay on one line, surrounding single or double quotes are stripped, and escape sequences like `\n` or `\"` are not interpreted.

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
| `youtrack_search` | Search tickets by free text or YouTrack query syntax and return structured issue data |
| `youtrack_fetch` | Fetch a ticket as structured JSON; attachments can be inlined or saved |
| `youtrack_fetch_markdown` | Fetch a ticket as rendered Markdown; attachments can be inlined or saved |
| `youtrack_get_statuses` | List valid statuses for a ticket |
| `youtrack_update_status` | Change a ticket status |
| `youtrack_create` | Create a ticket |
| `youtrack_update` | Update summary, description, or custom fields |
| `youtrack_link` | Link one ticket to another |
| `youtrack_unlink` | Remove a ticket link |

These are the default names. Set `YOUTRACK_TOOL_PREFIX=""` to expose the unprefixed variants instead.

All ticket-scoped tools require an explicit `ticket`. The MCP server always uses the configured `YOUTRACK_URL` and `YOUTRACK_API_TOKEN`; MCP calls cannot override them.

### Exposed MCP resources

| Resource | Purpose |
|---|---|
| `youtrack://{ticket}` | Read a ticket as structured JSON with attachments exported inline, for example `youtrack://YT-39` |
| `youtrack://{ticket}/markdown` | Read a ticket as Markdown with attachments exported inline, for example `youtrack://YT-39/markdown` |

## Common workflow

1. Build the binary.
2. Configure `YOUTRACK_URL` and `YOUTRACK_API_TOKEN`.
3. Run `install vscode` or `install copilot`.
4. Start a new IDE or Copilot session so the MCP config is picked up.
5. Use the configured tool names from your MCP-enabled client.

## Building

```bash
go build ./cmd/youtrack-mcp
```

For development details, architecture, and release/build notes, see [`DEV.md`](./DEV.md).
