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

Generate a Claude Code/Codex-style MCP configuration from the repo:

```bash
cd backend
go run ./cmd/allcallallctl mcp-config \
  -organization-id 1 \
  -user-id 7
```

The generated config has this shape:

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
        "MCP_USER_ID": "7",
        "AGENT_PROVIDER": "rules"
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

## AI Skill Demo

The repo-native Agent Skill Markdown is generated from the backend tool registry:

```bash
cd backend
go run ./cmd/allcallallctl skill
```

Use it as interview material or as the prompt-side contract for a Codex/Claude-style assistant. The generated text lists trigger patterns, read-only MCP tools, side-effect tools, and approval rules.
