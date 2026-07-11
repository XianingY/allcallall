from __future__ import annotations

from typing import Any


class MCPToolBridgeError(RuntimeError):
    pass


class MCPToolBridge:
    """Compatibility guard for the removed in-process MCP transport.

    MCP discovery and execution require a run-scoped capability and must use
    ``GoToolBridge``. Only the isolated Sandbox Runner may start stdio servers.
    """

    def __init__(
        self,
        name: str,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
    ) -> None:
        del command, args, env
        self.name = name

    def connect(self) -> None:
        raise MCPToolBridgeError("local MCP processes are disabled; use the Go tool gateway")

    def disconnect(self) -> None:
        return

    def list_tools(self) -> list[dict[str, Any]]:
        return []

    def execute_tool(self, tool_name: str, arguments: dict[str, Any]) -> str:
        del tool_name, arguments
        raise MCPToolBridgeError("direct MCP execution is disabled; use the Go tool gateway")
