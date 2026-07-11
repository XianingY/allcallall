from __future__ import annotations

import os
import yaml
from typing import Any
from pydantic import BaseModel, Field
from app.config import config
from app.tools.mcp_client import MCPToolBridge

class MCPServerConfig(BaseModel):
    name: str
    command: str
    args: list[str] = Field(default_factory=list)
    env: dict[str, str] = Field(default_factory=dict)

class AgentSkill(BaseModel):
    name: str
    description: str
    system_prompt_extension: str = ""
    mcp_servers: list[MCPServerConfig] = Field(default_factory=list)

class SkillManager:
    _instance: SkillManager | None = None

    def __init__(self) -> None:
        self.skills: dict[str, AgentSkill] = {}
        self.mcp_bridges: dict[str, MCPToolBridge] = {}
        self.tool_to_server: dict[str, str] = {}

    @classmethod
    def get_instance(cls) -> SkillManager:
        if cls._instance is None:
            cls._instance = SkillManager()
            cls._instance.load_skills()
        return cls._instance

    def load_skills(self) -> None:
        skills_dir = getattr(config, "skills_dir", os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), "skills"))
        if not os.path.exists(skills_dir):
            return

        for filename in os.listdir(skills_dir):
            if filename.endswith(".yaml") or filename.endswith(".yml"):
                path = os.path.join(skills_dir, filename)
                try:
                    with open(path, "r", encoding="utf-8") as f:
                        data = yaml.safe_load(f)
                        skill = AgentSkill(**data)
                        self.skills[skill.name] = skill

                        for srv_cfg in skill.mcp_servers:
                            if srv_cfg.name not in self.mcp_bridges:
                                self.mcp_bridges[srv_cfg.name] = MCPToolBridge(
                                    name=srv_cfg.name,
                                    command=srv_cfg.command,
                                    args=srv_cfg.args,
                                    env=srv_cfg.env,
                                )
                except Exception as e:
                    import logging
                    logging.getLogger(__name__).error(f"Failed to load skill {filename}: {e}")

    def connect_mcp_servers(self) -> None:
        for bridge in self.mcp_bridges.values():
            bridge.connect()

    def disconnect_mcp_servers(self) -> None:
        for bridge in self.mcp_bridges.values():
            bridge.disconnect()

    def get_active_system_prompts(self) -> str:
        parts = []
        for skill in self.skills.values():
            if skill.system_prompt_extension:
                parts.append(f"[{skill.name} Skill]\n{skill.system_prompt_extension}")

        # Inject tool schemas so LLM knows how to call them
        tools = self.get_all_mcp_tools()
        if tools:
            parts.append("\n[Available MCP Tools]")
            for t in tools:
                import json
                parts.append(f"- {t['name']}: {t['description']}\n  Schema: {json.dumps(t['input_schema'])}")

        return "\n\n".join(parts)

    def get_all_mcp_tools(self) -> list[dict[str, Any]]:
        all_tools = []
        for bridge in self.mcp_bridges.values():
            tools = bridge.list_tools()
            for t in tools:
                self.tool_to_server[t["name"]] = bridge.name
                all_tools.append(t)
        return all_tools

    def execute_mcp_tool(self, tool_name: str, arguments: dict[str, Any]) -> str:
        server_name = self.tool_to_server.get(tool_name)
        if not server_name:
            raise ValueError(f"Unknown MCP tool: {tool_name}")
        bridge = self.mcp_bridges.get(server_name)
        if not bridge:
            raise ValueError(f"MCP server {server_name} not found")
        return bridge.execute_tool(tool_name, arguments)
