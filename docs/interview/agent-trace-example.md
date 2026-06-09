# Agent Trace Example

Agent trace is generated from persisted facts: `agent_runs`, `agent_steps`, and `agent_tool_calls`. It does not require a separate trace table. This makes the execution explainable even after async workers, retries, or API clients disconnect.

## Tool Registry Snapshot

The registry lives in `backend/internal/agent/tool_registry.go`.

| Tool | Kind | Permission | Purpose |
| --- | --- | --- | --- |
| `query_recent_meetings` | `read_only` | `conversation_member` | Load recent room context. |
| `query_conversation_members` | `read_only` | `conversation_member` | Load bounded member context. |
| `query_contact_profile` | `read_only` | `conversation_member` | Load organization-scoped contact metadata. |
| `write_conversation_message` | `side_effect` | `conversation_writer` | Write Agent result into the thread and enqueue message events. |
| `create_follow_up_task` | `side_effect` | `conversation_writer` | Create a backend-owned follow-up task. |
| `upsert_agent_memory` | `side_effect` | `conversation_writer` | Store conversation-scoped Agent memory. |

## API Response Shape

`GET /api/v1/agent/runs/:id` returns the original `run`, `steps`, and `tool_calls`, plus a flattened `trace` array:

```json
{
  "run": {
    "id": 1,
    "source": "rules",
    "status": "ready",
    "summary": "APAC onboarding escalation 当前状态为 open，优先级为 high。",
    "next_step": "安排下一次会议或回访，并在线程内同步时间。"
  },
  "trace": [
    {
      "type": "run",
      "name": "agent.run.created",
      "status": "pending",
      "ref_id": 1,
      "metadata": {
        "conversation_id": 10,
        "organization_id": 2,
        "request_id": "req-demo-1",
        "source": "rules"
      }
    },
    {
      "type": "run",
      "name": "agent.run.started",
      "status": "running",
      "ref_id": 1,
      "metadata": {
        "attempts": 1
      }
    },
    {
      "type": "step",
      "name": "collect_context",
      "status": "ready",
      "ref_id": 11
    },
    {
      "type": "tool",
      "name": "query_recent_meetings",
      "status": "ready",
      "ref_id": 21,
      "metadata": {
        "kind": "read_only",
        "permission": "conversation_member"
      }
    },
    {
      "type": "step",
      "name": "plan_next_actions",
      "status": "ready",
      "ref_id": 12
    },
    {
      "type": "tool",
      "name": "write_conversation_message",
      "status": "ready",
      "ref_id": 24,
      "metadata": {
        "kind": "side_effect",
        "permission": "conversation_writer"
      }
    },
    {
      "type": "tool",
      "name": "create_follow_up_task",
      "status": "ready",
      "ref_id": 25,
      "metadata": {
        "kind": "side_effect",
        "permission": "conversation_writer"
      }
    },
    {
      "type": "tool",
      "name": "upsert_agent_memory",
      "status": "ready",
      "ref_id": 26,
      "metadata": {
        "kind": "side_effect",
        "permission": "conversation_writer"
      }
    },
    {
      "type": "run",
      "name": "agent.run.ready",
      "status": "ready",
      "ref_id": 1
    }
  ]
}
```

## How To Explain It

- `run` is the state machine record.
- `steps` are human-readable execution stages.
- `tool_calls` are auditable backend tool invocations with JSON input/output.
- `trace` is a presentation layer that sorts run/step/tool facts into one timeline.
- Side effects remain backend-owned. The planner returns structured intent; the service decides which tools run and records their output.
