import type {
  CreateMCPInstallationRequest,
  MCPInstallationDefinition,
} from "../api/mcpPlatform";

export type MCPSourceType = CreateMCPInstallationRequest["source_type"];
export type MCPScope = CreateMCPInstallationRequest["scope"];
export type MCPTransport = MCPInstallationDefinition["transport"];

export interface MCPInstallationDraft {
  displayName: string;
  scope: MCPScope;
  sourceType: MCPSourceType;
  transport: MCPTransport;
  imageRef: string;
  endpointURL: string;
  commandLines: string;
  argumentLines: string;
  allowlistLines: string;
}

export interface SecretDraft {
  key: string;
  value: string;
}

export interface ValidationResult<T> {
  value?: T;
  errors: Record<string, string>;
}

const OCI_DIGEST_PATTERN = /^.+@sha256:[a-fA-F0-9]{64}$/;

export const splitNonEmptyLines = (raw: string): string[] =>
  raw
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);

const isPrivateIPv4 = (hostname: string) => {
  const parts = hostname.split(".").map(Number);
  if (parts.length !== 4 || parts.some((part) => !Number.isInteger(part))) {
    return false;
  }
  const [first, second] = parts;
  return (
    first === 10 ||
    first === 127 ||
    (first === 169 && second === 254) ||
    (first === 172 && second >= 16 && second <= 31) ||
    (first === 192 && second === 168) ||
    first === 0
  );
};

export const isPublicHTTPSEndpoint = (raw: string): boolean => {
  try {
    const url = new URL(raw.trim());
    const hostname = url.hostname.toLowerCase().replace(/^\[|\]$/g, "");
    if (url.protocol !== "https:" || url.username || url.password) {
      return false;
    }
    if (
      !hostname ||
      hostname === "localhost" ||
      hostname === "::1" ||
      hostname.endsWith(".local") ||
      hostname.endsWith(".internal") ||
      (hostname.includes(":") &&
        (/^f[cd]/.test(hostname) || hostname.startsWith("fe80:"))) ||
      isPrivateIPv4(hostname)
    ) {
      return false;
    }
    return true;
  } catch {
    return false;
  }
};

export const validateInstallationDraft = (
  draft: MCPInstallationDraft,
): ValidationResult<CreateMCPInstallationRequest> => {
  const errors: Record<string, string> = {};
  const displayName = draft.displayName.trim();
  if (!displayName) {
    errors.displayName = "请输入连接名称";
  }

  if (draft.sourceType === "oci" && !OCI_DIGEST_PATTERN.test(draft.imageRef.trim())) {
    errors.imageRef = "OCI 镜像必须固定到 @sha256 digest";
  }
  if (
    draft.sourceType === "https" &&
    !isPublicHTTPSEndpoint(draft.endpointURL)
  ) {
    errors.endpointURL = "仅支持不含凭据的公网 HTTPS 地址";
  }

  if (Object.keys(errors).length > 0) {
    return { errors };
  }

  const command = splitNonEmptyLines(draft.commandLines);
  const args = splitNonEmptyLines(draft.argumentLines);
  const networkAllowlist = splitNonEmptyLines(draft.allowlistLines);
  const definition: MCPInstallationDefinition = {
    transport: draft.sourceType === "oci" ? "stdio" : draft.transport,
  };
  if (draft.sourceType === "oci") {
    definition.image_ref = draft.imageRef.trim();
    if (command.length > 0) definition.command = command;
    if (args.length > 0) definition.args = args;
  } else {
    definition.endpoint_url = draft.endpointURL.trim();
  }
  if (networkAllowlist.length > 0) {
    definition.network_allowlist = networkAllowlist;
  }

  return {
    errors,
    value: {
      ...definition,
      display_name: displayName,
      scope: draft.scope,
      source_type: draft.sourceType,
    },
  };
};

export const validateSecretDrafts = (
  drafts: SecretDraft[],
): ValidationResult<Record<string, string>> => {
  const errors: Record<string, string> = {};
  const secrets: Record<string, string> = {};
  drafts.forEach((draft, index) => {
    const key = draft.key.trim();
    if (!key || !draft.value.trim()) {
      errors[String(index)] = "Secret 名称和值不能为空";
      return;
    }
    if (Object.prototype.hasOwnProperty.call(secrets, key)) {
      errors[String(index)] = `Secret ${key} 重复`;
      return;
    }
    secrets[key] = draft.value;
  });
  if (drafts.length === 0) {
    errors.form = "至少添加一个 Secret";
  }
  return Object.keys(errors).length > 0
    ? { errors }
    : { errors, value: secrets };
};

export const formatPlatformJSON = (value: unknown): string => {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return "{}";
  }
};

export const canManageScopedResource = (
  scope: MCPScope,
  isOrganizationAdmin: boolean,
): boolean => scope === "personal" || isOrganizationAdmin;

export const canBindInstallationToSkill = (
  skillScope: MCPScope,
  installationScope: MCPScope,
): boolean => skillScope === "personal" || installationScope === "organization";

export const toolExecutionPolicy = (
  risk: "read" | "write" | "unknown",
): string => {
  if (risk === "read") return "已验证的只读工具可直接执行";
  if (risk === "write") return "写入操作需要人工审批";
  return "风险未分类，需要人工审批";
};
