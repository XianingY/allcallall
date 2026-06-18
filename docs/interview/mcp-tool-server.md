# MCP Tool Server

AllCallAll exposes a local MCP-compatible stdio server for read-only Agent tools.

## Run

```bash
cd backend
CONFIG_PATH=./configs/config.yaml \
MCP_ORGANIZATION_ID=1 \
MCP_USER_ID=7 \
go run ./cmd/mcp-tool-server
```

The server writes JSON-RPC responses to stdout and logs to stderr. Do not pipe app logs into stdout when using it from an MCP client.

## Client Entry

Use the command below in a Claude Code/Codex-style MCP configuration:

```json
{
  "mcpServers": {
    "allcallall": {
      "command": "go",
      "args": ["run", "./cmd/mcp-tool-server"],
      "cwd": "/Users/byzantium/github/AllCallAll/backend",
      "env": {
        "CONFIG_PATH": "./configs/config.yaml",
        "MCP_ORGANIZATION_ID": "1",
        "MCP_USER_ID": "7"
      }
    }
  }
}
```

## Exposed Tools

Only read-only tools are exposed in v1:

- `query_recent_meetings`
- `query_conversation_members`
- `query_contact_profile`
- `query_context_chunks`

Side-effect tools such as `write_conversation_message` are intentionally not exposed by this MCP server.
