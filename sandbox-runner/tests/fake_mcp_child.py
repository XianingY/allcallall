from __future__ import annotations

import json
import sys
from typing import Any


def send(payload: dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(payload, separators=(",", ":")) + "\n")
    sys.stdout.flush()


for raw_line in sys.stdin:
    message = json.loads(raw_line)
    request_id = message.get("id")
    method = message.get("method")
    if method == "initialize":
        requested_version = message.get("params", {}).get("protocolVersion", "2024-11-05")
        send(
            {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "protocolVersion": requested_version,
                    "capabilities": {"tools": {"listChanged": False}},
                    "serverInfo": {"name": "fake-stdio-child", "version": "1.0.0"},
                },
            }
        )
    elif method == "tools/list":
        send(
            {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "tools": [
                        {
                            "name": "echo",
                            "description": "Return a fixed contract-test response.",
                            "inputSchema": {"type": "object"},
                            "annotations": {"readOnlyHint": True},
                        }
                    ]
                },
            }
        )
    elif method == "tools/call":
        send(
            {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "content": [{"type": "text", "text": "remote-contract-ok"}],
                    "structuredContent": {"ok": True},
                    "isError": False,
                },
            }
        )
