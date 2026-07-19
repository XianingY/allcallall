from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field


class InstallationDefinition(BaseModel):
    model_config = ConfigDict(extra="forbid")

    transport: str
    image_ref: str = ""
    endpoint_url: str = ""
    command: list[str] = Field(default_factory=list)
    args: list[str] = Field(default_factory=list)
    config: dict[str, Any] = Field(default_factory=dict)
    network_allowlist: list[str] = Field(default_factory=list)


class ValidationRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    installation_id: int
    revision_id: int
    source_type: Literal["oci", "https"]
    definition: InstallationDefinition
    secret_wrap_token: str = ""


class ExecutionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    execution_id: str
    organization_id: int
    user_id: int
    conversation_id: int
    run_id: int
    run_ref: str = ""
    tool_call_id: str = ""
    installation_id: int
    revision_id: int
    tool_id: int = 0
    source_type: Literal["oci", "https"]
    definition: InstallationDefinition
    tool_name: str
    arguments: dict[str, Any] = Field(default_factory=dict)
    secret_wrap_token: str = ""
    timeout_ms: int = 30_000
    output_limit: int = 256 * 1024


class DiscoveredTool(BaseModel):
    name: str
    description: str = ""
    input_schema: dict[str, Any] = Field(default_factory=dict)
    output_schema: dict[str, Any] = Field(default_factory=dict)
    risk: Literal["read", "write", "unknown"] = "unknown"
    schema_version: str = "mcp-1"


class ValidationResponse(BaseModel):
    image_digest: str = ""
    scan_status: str = "passed"
    scan_report: dict[str, Any] = Field(default_factory=dict)
    tools: list[DiscoveredTool] = Field(default_factory=list)


class ExecutionResponse(BaseModel):
    job_id: str
    output: dict[str, Any]
