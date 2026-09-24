import React from "react";
import { Pressable, Text, View } from "react-native";

import type { MCPInstallation, MCPTool } from "../../api/mcpPlatform";
import {
  activateMCPInstallation,
  publishMCPInstallation,
  validateMCPInstallation
} from "../../api/mcpPlatform";
import {
  toolExecutionPolicy,
  type MCPInstallationDraft,
  type MCPScope,
  type MCPSourceType,
  type MCPTransport,
  type SecretDraft
} from "../mcpPlatformUtils";

import { ActionButton, Field, SegmentedControl, StatusBadge } from "./primitives";
import { SecretEditor } from "./SecretEditor";
import { styles } from "./styles";

interface InstallationsPanelProps {
  installations: MCPInstallation[];
  selectedInstallation: MCPInstallation | null;
  onSelectInstallation: (installation: MCPInstallation) => void;
  showInstaller: boolean;
  setShowInstaller: React.Dispatch<React.SetStateAction<boolean>>;
  installationDraft: MCPInstallationDraft;
  setInstallationDraft: React.Dispatch<React.SetStateAction<MCPInstallationDraft>>;
  installationErrors: Record<string, string>;
  onCreateInstallation: () => void;
  isAdmin: boolean;
  busyKey: string;
  revisionLabel: string;
  canManageSelectedInstallation: boolean;
  runInstallationAction: (
    key: string,
    action: () => Promise<MCPInstallation>,
    success: string
  ) => void;
  token: string | null;
  onDisableInstallation: () => void;
  secretDrafts: SecretDraft[];
  setSecretDrafts: React.Dispatch<React.SetStateAction<SecretDraft[]>>;
  secretErrors: Record<string, string>;
  onUpdateSecretRow: (index: number, patch: Partial<SecretDraft>) => void;
  onSaveSecrets: () => void;
  tools: MCPTool[];
  selectedToolIds: number[];
  onToggleTool: (toolID: number) => void;
}

export const InstallationsPanel: React.FC<InstallationsPanelProps> = ({
  installations,
  selectedInstallation,
  onSelectInstallation,
  showInstaller,
  setShowInstaller,
  installationDraft,
  setInstallationDraft,
  installationErrors,
  onCreateInstallation,
  isAdmin,
  busyKey,
  revisionLabel,
  canManageSelectedInstallation,
  runInstallationAction,
  token,
  onDisableInstallation,
  secretDrafts,
  setSecretDrafts,
  secretErrors,
  onUpdateSecretRow,
  onSaveSecrets,
  tools,
  selectedToolIds,
  onToggleTool
}) => (
          <>
            <View style={styles.sectionHeader}>
              <View>
                <Text style={styles.sectionTitle}>MCP 安装</Text>
                <Text style={styles.sectionMeta}>
                  {installations.length} / {isAdmin ? "组织管理员" : "个人成员"}
                </Text>
              </View>
              <ActionButton
                label={showInstaller ? "收起" : "新增"}
                compact
                onPress={() => setShowInstaller((current) => !current)}
              />
            </View>

            {showInstaller ? (
              <View style={styles.formBand}>
                <Text style={styles.formTitle}>新安装</Text>
                <Text style={styles.fieldLabel}>来源</Text>
                <SegmentedControl<MCPSourceType>
                  value={installationDraft.sourceType}
                  onChange={(sourceType) =>
                    setInstallationDraft((current) => ({
                      ...current,
                      sourceType,
                      transport:
                        sourceType === "oci" ? "stdio" : "streamable_http",
                    }))
                  }
                  options={[
                    { value: "https", label: "HTTPS" },
                    { value: "oci", label: "OCI" },
                  ]}
                />
                <Text style={styles.fieldLabel}>作用域</Text>
                <SegmentedControl<MCPScope>
                  value={installationDraft.scope}
                  onChange={(scope) =>
                    setInstallationDraft((current) => ({ ...current, scope }))
                  }
                  options={[
                    { value: "personal", label: "个人" },
                    { value: "organization", label: "组织" },
                  ]}
                />
                {!isAdmin && installationDraft.scope === "organization" ? (
                  <Text style={styles.warningText}>组织安装需要管理员权限</Text>
                ) : null}
                <Field
                  label="连接名称"
                  value={installationDraft.displayName}
                  error={installationErrors.displayName}
                  onChangeText={(displayName) =>
                    setInstallationDraft((current) => ({ ...current, displayName }))
                  }
                  placeholder="例如 Calendar MCP"
                />
                {installationDraft.sourceType === "https" ? (
                  <>
                    <Text style={styles.fieldLabel}>传输协议</Text>
                    <SegmentedControl<MCPTransport>
                      value={installationDraft.transport}
                      onChange={(transport) =>
                        setInstallationDraft((current) => ({ ...current, transport }))
                      }
                      options={[
                        { value: "streamable_http", label: "Streamable" },
                        { value: "http", label: "HTTP" },
                        { value: "sse", label: "SSE" },
                      ]}
                    />
                    <Field
                      label="HTTPS Endpoint"
                      value={installationDraft.endpointURL}
                      error={installationErrors.endpointURL}
                      autoCapitalize="none"
                      onChangeText={(endpointURL) =>
                        setInstallationDraft((current) => ({ ...current, endpointURL }))
                      }
                      placeholder="https://mcp.example.com/v1"
                    />
                  </>
                ) : (
                  <>
                    <Field
                      label="OCI Image Digest"
                      value={installationDraft.imageRef}
                      error={installationErrors.imageRef}
                      autoCapitalize="none"
                      onChangeText={(imageRef) =>
                        setInstallationDraft((current) => ({ ...current, imageRef }))
                      }
                      placeholder="registry/image@sha256:..."
                    />
                    <Field
                      label="Command（每行一项）"
                      value={installationDraft.commandLines}
                      multiline
                      autoCapitalize="none"
                      onChangeText={(commandLines) =>
                        setInstallationDraft((current) => ({ ...current, commandLines }))
                      }
                    />
                    <Field
                      label="Arguments（每行一项）"
                      value={installationDraft.argumentLines}
                      multiline
                      autoCapitalize="none"
                      onChangeText={(argumentLines) =>
                        setInstallationDraft((current) => ({ ...current, argumentLines }))
                      }
                    />
                  </>
                )}
                <Field
                  label="Network Allowlist（每行一个域名）"
                  value={installationDraft.allowlistLines}
                  multiline
                  autoCapitalize="none"
                  onChangeText={(allowlistLines) =>
                    setInstallationDraft((current) => ({ ...current, allowlistLines }))
                  }
                />
                <ActionButton
                  label={busyKey === "create-installation" ? "创建中..." : "创建安装"}
                  disabled={busyKey !== "" || (!isAdmin && installationDraft.scope === "organization")}
                  onPress={() => void onCreateInstallation()}
                />
              </View>
            ) : null}

            <View style={styles.installationLayout}>
              <View style={styles.listPane}>
                {installations.length === 0 ? (
                  <Text style={styles.emptyInline}>暂无可见安装</Text>
                ) : (
                  installations.map((installation) => (
                    <Pressable
                      key={installation.id}
                      onPress={() => void onSelectInstallation(installation)}
                      style={[
                        styles.itemCard,
                        selectedInstallation?.id === installation.id &&
                          styles.itemCardSelected,
                      ]}
                    >
                      <View style={styles.rowBetween}>
                        <Text style={styles.itemTitle}>{installation.display_name}</Text>
                        <StatusBadge status={installation.status} />
                      </View>
                      <Text style={styles.itemMeta}>
                        {installation.source_type.toUpperCase()} · {installation.scope} · #{installation.id}
                      </Text>
                    </Pressable>
                  ))
                )}
              </View>

              {selectedInstallation ? (
                <View style={styles.detailPane}>
                  <View style={styles.rowBetween}>
                    <View style={styles.flexCopy}>
                      <Text style={styles.detailTitle}>{selectedInstallation.display_name}</Text>
                      <Text style={styles.itemMeta}>{revisionLabel}</Text>
                    </View>
                    <StatusBadge status={selectedInstallation.status} />
                  </View>
                  <View style={styles.detailGrid}>
                    <Text style={styles.detailKey}>来源</Text>
                    <Text style={styles.detailValue}>{selectedInstallation.source_type.toUpperCase()}</Text>
                    <Text style={styles.detailKey}>作用域</Text>
                    <Text style={styles.detailValue}>{selectedInstallation.scope}</Text>
                    <Text style={styles.detailKey}>凭据</Text>
                    <Text style={styles.detailValue}>
                      {selectedInstallation.secrets_configured ? "已配置" : "未配置"}
                    </Text>
                  </View>
                  {selectedInstallation.published_at ? (
                    <Text style={styles.itemMeta}>
                      组织发布于 {new Date(selectedInstallation.published_at).toLocaleString()}
                    </Text>
                  ) : null}
                  {selectedInstallation.latest_revision?.image_digest ? (
                    <Text style={styles.monospace} numberOfLines={2}>
                      {selectedInstallation.latest_revision.image_digest}
                    </Text>
                  ) : null}
                  {selectedInstallation.latest_revision?.endpoint_url ? (
                    <Text style={styles.monospace} numberOfLines={2}>
                      {selectedInstallation.latest_revision.endpoint_url}
                    </Text>
                  ) : null}
                  {selectedInstallation.last_error ? (
                    <Text style={styles.warningText}>{selectedInstallation.last_error}</Text>
                  ) : null}
                  {!canManageSelectedInstallation ? (
                    <Text style={styles.warningText}>
                      组织安装仅 owner 或 admin 可修改；你仍可查看并绑定其工具。
                    </Text>
                  ) : null}
                  <View style={styles.actionRow}>
                    <ActionButton
                      label="验证连接"
                      compact
                      disabled={busyKey !== "" || !canManageSelectedInstallation}
                      onPress={() =>
                        void runInstallationAction(
                          "validate",
                          () => validateMCPInstallation(token!, selectedInstallation.id),
                          "连接验证完成",
                        )
                      }
                    />
                    <ActionButton
                      label="激活"
                      compact
                      disabled={
                        busyKey !== "" ||
                        !canManageSelectedInstallation ||
                        selectedInstallation.status !== "disabled"
                      }
                      onPress={() =>
                        void runInstallationAction(
                          "activate",
                          () => activateMCPInstallation(token!, selectedInstallation.id),
                          "安装已激活",
                        )
                      }
                    />
                    {selectedInstallation.scope === "personal" ? (
                      <ActionButton
                        label="发布到组织"
                        compact
                        disabled={
                          busyKey !== "" ||
                          !canManageSelectedInstallation ||
                          !isAdmin ||
                          selectedInstallation.status !== "active"
                        }
                        onPress={() =>
                          void runInstallationAction(
                            "publish",
                            () => publishMCPInstallation(token!, selectedInstallation.id),
                            "Revision 已发布到组织",
                          )
                        }
                      />
                    ) : null}
                    <ActionButton
                      label="禁用"
                      compact
                      danger
                      disabled={busyKey !== "" || !canManageSelectedInstallation}
                      onPress={onDisableInstallation}
                    />
                  </View>

                  <SecretEditor
                    secretDrafts={secretDrafts}
                    setSecretDrafts={setSecretDrafts}
                    secretErrors={secretErrors}
                    canManageSelectedInstallation={canManageSelectedInstallation}
                    onUpdateSecretRow={onUpdateSecretRow}
                    busyKey={busyKey}
                    onSaveSecrets={onSaveSecrets}
                  />

                  <View style={styles.divider} />
                  <Text style={styles.formTitle}>工具目录</Text>
                  {tools.length === 0 ? (
                    <Text style={styles.emptyInline}>验证后将显示工具</Text>
                  ) : (
                    tools.map((tool) => (
                      <Pressable
                        key={tool.id}
                        onPress={() => onToggleTool(tool.id)}
                        style={[
                          styles.toolRow,
                          selectedToolIds.includes(tool.id) && styles.toolRowSelected,
                        ]}
                      >
                        <View style={styles.flexCopy}>
                          <Text style={styles.toolName}>{tool.name}</Text>
                          <Text style={styles.itemMeta}>
                            Revision #{tool.revision_id} · schema {tool.schema_version}
                          </Text>
                          {tool.description ? (
                            <Text style={styles.toolDescription}>{tool.description}</Text>
                          ) : null}
                        </View>
                        <View>
                          <StatusBadge status={tool.risk} />
                          <Text style={styles.approvalReason}>
                            {toolExecutionPolicy(tool.risk)}
                          </Text>
                        </View>
                      </Pressable>
                    ))
                  )}
                </View>
              ) : (
                <View style={styles.detailPane}>
                  <Text style={styles.emptyInline}>选择安装查看 revision、凭据和工具</Text>
                </View>
              )}
            </View>
          </>
);
