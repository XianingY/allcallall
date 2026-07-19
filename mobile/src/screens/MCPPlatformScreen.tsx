import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Platform,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import axios from "axios";

import {
  activateMCPInstallation,
  createAgentSkill,
  createMCPInstallation,
  deleteAgentSkill,
  disableMCPInstallation,
  getMCPExecution,
  getMCPInstallation,
  listAgentSkills,
  listMCPInstallations,
  listMCPInstallationTools,
  publishMCPInstallation,
  putMCPInstallationSecrets,
  updateAgentSkill,
  validateMCPInstallation,
  type AgentSkill,
  type MCPExecution,
  type MCPInstallation,
  type MCPTool,
} from "../api/mcpPlatform";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import type { RootStackParamList } from "../navigation/AppNavigator";
import {
  canBindInstallationToSkill,
  canManageScopedResource,
  formatPlatformJSON,
  toolExecutionPolicy,
  validateInstallationDraft,
  validateSecretDrafts,
  type MCPInstallationDraft,
  type MCPScope,
  type MCPSourceType,
  type MCPTransport,
  type SecretDraft,
} from "./mcpPlatformUtils";

type Props = NativeStackScreenProps<RootStackParamList, "MCPPlatform">;
type PlatformTab = "installations" | "skills" | "execution";

const EMPTY_INSTALLATION: MCPInstallationDraft = {
  displayName: "",
  scope: "personal",
  sourceType: "https",
  transport: "streamable_http",
  imageRef: "",
  endpointURL: "",
  commandLines: "",
  argumentLines: "",
  allowlistLines: "",
};

const STATUS_COLORS: Record<string, { background: string; foreground: string }> = {
  active: { background: "#dcfce7", foreground: "#166534" },
  validating: { background: "#dbeafe", foreground: "#1d4ed8" },
  quarantined: { background: "#ffedd5", foreground: "#9a3412" },
  failed: { background: "#fee2e2", foreground: "#991b1b" },
  disabled: { background: "#f3f4f6", foreground: "#4b5563" },
  draft: { background: "#fef3c7", foreground: "#92400e" },
  succeeded: { background: "#dcfce7", foreground: "#166534" },
  running: { background: "#dbeafe", foreground: "#1d4ed8" },
  timed_out: { background: "#fee2e2", foreground: "#991b1b" },
};

const errorMessage = (error: unknown) => {
  if (axios.isAxiosError(error)) {
    const payload = error.response?.data as
      | { error?: string; message?: string }
      | undefined;
    return payload?.message || payload?.error || error.message;
  }
  return error instanceof Error ? error.message : "请求失败，请稍后重试";
};

const ActionButton: React.FC<{
  label: string;
  onPress: () => void;
  disabled?: boolean;
  danger?: boolean;
  compact?: boolean;
}> = ({ label, onPress, disabled, danger, compact }) => (
  <Pressable
    accessibilityRole="button"
    disabled={disabled}
    onPress={onPress}
    style={({ pressed }) => [
      styles.actionButton,
      compact && styles.actionButtonCompact,
      danger && styles.actionButtonDanger,
      disabled && styles.actionButtonDisabled,
      pressed && !disabled && styles.actionButtonPressed,
    ]}
  >
    <Text style={[styles.actionButtonText, danger && styles.actionButtonDangerText]}>
      {label}
    </Text>
  </Pressable>
);

const Field: React.FC<{
  label: string;
  value: string;
  onChangeText: (value: string) => void;
  placeholder?: string;
  error?: string;
  multiline?: boolean;
  secureTextEntry?: boolean;
  editable?: boolean;
  autoCapitalize?: "none" | "sentences" | "words" | "characters";
}> = ({ label, error, multiline, ...props }) => (
  <View style={styles.field}>
    <Text style={styles.fieldLabel}>{label}</Text>
    <TextInput
      {...props}
      autoCorrect={false}
      multiline={multiline}
      placeholderTextColor="#9ca3af"
      style={[
        styles.input,
        multiline && styles.multilineInput,
        Boolean(error) && styles.inputError,
      ]}
    />
    {error ? <Text style={styles.errorText}>{error}</Text> : null}
  </View>
);

const SegmentedControl = <T extends string>({
  value,
  options,
  onChange,
}: {
  value: T;
  options: Array<{ value: T; label: string }>;
  onChange: (value: T) => void;
}) => (
  <View style={styles.segmentedControl}>
    {options.map((option) => (
      <Pressable
        key={option.value}
        accessibilityRole="button"
        accessibilityState={{ selected: value === option.value }}
        onPress={() => onChange(option.value)}
        style={[
          styles.segment,
          value === option.value && styles.segmentSelected,
        ]}
      >
        <Text
          style={[
            styles.segmentText,
            value === option.value && styles.segmentTextSelected,
          ]}
        >
          {option.label}
        </Text>
      </Pressable>
    ))}
  </View>
);

const StatusBadge: React.FC<{ status: string }> = ({ status }) => {
  const color = STATUS_COLORS[status] ?? {
    background: "#f3f4f6",
    foreground: "#374151",
  };
  return (
    <View style={[styles.badge, { backgroundColor: color.background }]}>
      <Text style={[styles.badgeText, { color: color.foreground }]}>{status}</Text>
    </View>
  );
};

const MCPPlatformScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const [activeTab, setActiveTab] = useState<PlatformTab>("installations");
  const [installations, setInstallations] = useState<MCPInstallation[]>([]);
  const [skills, setSkills] = useState<AgentSkill[]>([]);
  const [selectedInstallation, setSelectedInstallation] =
    useState<MCPInstallation | null>(null);
  const [tools, setTools] = useState<MCPTool[]>([]);
  const [selectedToolIds, setSelectedToolIds] = useState<number[]>([]);
  const [installationDraft, setInstallationDraft] =
    useState<MCPInstallationDraft>(EMPTY_INSTALLATION);
  const [installationErrors, setInstallationErrors] = useState<
    Record<string, string>
  >({});
  const [showInstaller, setShowInstaller] = useState(false);
  const [secretDrafts, setSecretDrafts] = useState<SecretDraft[]>([
    { key: "", value: "" },
  ]);
  const [secretErrors, setSecretErrors] = useState<Record<string, string>>({});
  const [skillName, setSkillName] = useState("");
  const [skillDescription, setSkillDescription] = useState("");
  const [skillInstructions, setSkillInstructions] = useState("");
  const [skillScope, setSkillScope] = useState<MCPScope>("personal");
  const [executionID, setExecutionID] = useState("");
  const [execution, setExecution] = useState<MCPExecution | null>(null);
  const [executionInstallation, setExecutionInstallation] =
    useState<MCPInstallation | null>(null);
  const [executionTool, setExecutionTool] = useState<MCPTool | null>(null);
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");

  const isAdmin =
    currentOrganization?.role === "owner" ||
    currentOrganization?.role === "admin";
  const canManageSelectedInstallation = selectedInstallation
    ? canManageScopedResource(selectedInstallation.scope, isAdmin)
    : false;
  const canBindSelectedToolsToSkill = selectedInstallation
    ? canBindInstallationToSkill(skillScope, selectedInstallation.scope)
    : false;

  const refresh = useCallback(async () => {
    if (!token || !currentOrganization) return;
    setBusyKey("refresh");
    try {
      const [nextInstallations, nextSkills] = await Promise.all([
        listMCPInstallations(token),
        listAgentSkills(token),
      ]);
      setInstallations(nextInstallations);
      setSkills(nextSkills);
      setNotice("");
      if (selectedInstallation) {
        const stillVisible = nextInstallations.find(
          (item) => item.id === selectedInstallation.id,
        );
        if (!stillVisible) {
          setSelectedInstallation(null);
          setTools([]);
          setSelectedToolIds([]);
        }
      }
    } catch (error) {
      setNotice(errorMessage(error));
    } finally {
      setBusyKey("");
    }
  }, [currentOrganization, selectedInstallation, token]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const selectInstallation = useCallback(
    async (installation: MCPInstallation) => {
      if (!token) return;
      setBusyKey(`select:${installation.id}`);
      try {
        const [detail, nextTools] = await Promise.all([
          getMCPInstallation(token, installation.id),
          listMCPInstallationTools(token, installation.id),
        ]);
        setSelectedInstallation(detail);
        setTools(nextTools);
        setSelectedToolIds([]);
        setNotice("");
      } catch (error) {
        setNotice(errorMessage(error));
      } finally {
        setBusyKey("");
      }
    },
    [token],
  );

  const replaceInstallation = useCallback((next: MCPInstallation) => {
    setSelectedInstallation((current) =>
      current?.id === next.id ? { ...current, ...next } : current,
    );
    setInstallations((current) =>
      current.map((item) => (item.id === next.id ? { ...item, ...next } : item)),
    );
  }, []);

  const runInstallationAction = useCallback(
    async (
      key: string,
      action: () => Promise<MCPInstallation>,
      success: string,
    ) => {
      setBusyKey(key);
      try {
        const next = await action();
        replaceInstallation(next);
        setNotice(success);
        if (token) {
          const detail = await getMCPInstallation(token, next.id);
          replaceInstallation(detail);
          setSelectedInstallation(detail);
          setTools(await listMCPInstallationTools(token, next.id));
        }
      } catch (error) {
        setNotice(errorMessage(error));
      } finally {
        setBusyKey("");
      }
    },
    [replaceInstallation, token],
  );

  const handleCreateInstallation = useCallback(async () => {
    if (!token) return;
    const result = validateInstallationDraft(installationDraft);
    setInstallationErrors(result.errors);
    if (!result.value) return;
    setBusyKey("create-installation");
    try {
      const installation = await createMCPInstallation(token, result.value);
      setInstallationDraft(EMPTY_INSTALLATION);
      setShowInstaller(false);
      setInstallations((current) => [installation, ...current]);
      setNotice("安装已创建，可以开始连接验证");
      await selectInstallation(installation);
    } catch (error) {
      setNotice(errorMessage(error));
    } finally {
      setBusyKey("");
    }
  }, [installationDraft, selectInstallation, token]);

  const confirmDisableInstallation = useCallback(() => {
    if (!token || !selectedInstallation) return;
    const perform = async () => {
      setBusyKey("disable-installation");
      try {
        await disableMCPInstallation(token, selectedInstallation.id);
        setInstallations((current) =>
          current.filter((item) => item.id !== selectedInstallation.id),
        );
        setSelectedInstallation(null);
        setTools([]);
        setSelectedToolIds([]);
        setNotice("安装已禁用，关联凭据已撤销");
      } catch (error) {
        setNotice(errorMessage(error));
      } finally {
        setBusyKey("");
      }
    };
    const message = "禁用后工具将立即从 Agent 目录移除，关联凭据也会被撤销。";
    if (Platform.OS === "web" && typeof window !== "undefined") {
      if (window.confirm(message)) void perform();
      return;
    }
    Alert.alert("禁用安装", message, [
      { text: "取消", style: "cancel" },
      { text: "禁用", style: "destructive", onPress: () => void perform() },
    ]);
  }, [selectedInstallation, token]);

  const updateSecretRow = (index: number, patch: Partial<SecretDraft>) => {
    setSecretDrafts((current) =>
      current.map((item, itemIndex) =>
        itemIndex === index ? { ...item, ...patch } : item,
      ),
    );
  };

  const handleSaveSecrets = useCallback(async () => {
    if (!token || !selectedInstallation) return;
    const result = validateSecretDrafts(secretDrafts);
    setSecretErrors(result.errors);
    if (!result.value) return;
    const values = result.value;
    setBusyKey("save-secrets");
    try {
      await putMCPInstallationSecrets(token, selectedInstallation.id, values);
      replaceInstallation({
        ...selectedInstallation,
        secrets_configured: true,
      });
      setSecretDrafts([{ key: "", value: "" }]);
      setNotice("Secret 已安全写入凭据服务");
      setSecretErrors({});
    } catch (error) {
      setNotice(errorMessage(error));
    } finally {
      setBusyKey("");
    }
  }, [replaceInstallation, secretDrafts, selectedInstallation, token]);

  const toggleTool = (toolID: number) => {
    setSelectedToolIds((current) =>
      current.includes(toolID)
        ? current.filter((item) => item !== toolID)
        : [...current, toolID],
    );
  };

  const handleCreateSkill = useCallback(async () => {
    if (!token) return;
    const name = skillName.trim();
    const instructions = skillInstructions.trim();
    if (!name || !instructions) {
      setNotice("Skill 名称和指令不能为空");
      return;
    }
    if (selectedToolIds.length === 0) {
      setNotice("请至少绑定一个已验证的 MCP 工具");
      return;
    }
    if (!canBindSelectedToolsToSkill) {
      setNotice("组织 Skill 只能绑定已发布到组织的工具");
      return;
    }
    setBusyKey("create-skill");
    try {
      const skill = await createAgentSkill(token, {
        scope: skillScope,
        name,
        description: skillDescription.trim(),
        instructions,
        tool_ids: selectedToolIds,
      });
      setSkills((current) => [skill, ...current]);
      setSkillName("");
      setSkillDescription("");
      setSkillInstructions("");
      setSelectedToolIds([]);
      setNotice("Skill 已创建");
    } catch (error) {
      setNotice(errorMessage(error));
    } finally {
      setBusyKey("");
    }
  }, [
    selectedToolIds,
    skillDescription,
    skillInstructions,
    skillName,
    skillScope,
    canBindSelectedToolsToSkill,
    token,
  ]);

  const toggleSkillStatus = useCallback(
    async (skill: AgentSkill) => {
      if (!token) return;
      setBusyKey(`skill:${skill.id}`);
      try {
        const next = await updateAgentSkill(token, skill.id, {
          status: skill.status === "active" ? "disabled" : "active",
        });
        setSkills((current) =>
          current.map((item) => (item.id === next.id ? next : item)),
        );
        setNotice(`Skill 已${next.status === "active" ? "启用" : "停用"}`);
      } catch (error) {
        setNotice(errorMessage(error));
      } finally {
        setBusyKey("");
      }
    },
    [token],
  );

  const updateSkillBinding = useCallback(
    async (skill: AgentSkill) => {
      if (!token) return;
      setBusyKey(`bind-skill:${skill.id}`);
      try {
        const next = await updateAgentSkill(token, skill.id, {
          tool_ids: selectedToolIds,
        });
        setSkills((current) =>
          current.map((item) => (item.id === next.id ? next : item)),
        );
        setNotice(`Skill 绑定已更新为 ${selectedToolIds.length} 个工具`);
      } catch (error) {
        setNotice(errorMessage(error));
      } finally {
        setBusyKey("");
      }
    },
    [selectedToolIds, token],
  );

  const removeSkill = useCallback(
    async (skill: AgentSkill) => {
      if (!token) return;
      setBusyKey(`delete-skill:${skill.id}`);
      try {
        await deleteAgentSkill(token, skill.id);
        setSkills((current) => current.filter((item) => item.id !== skill.id));
        setNotice("Skill 已删除");
      } catch (error) {
        setNotice(errorMessage(error));
      } finally {
        setBusyKey("");
      }
    },
    [token],
  );

  const handleFetchExecution = useCallback(async () => {
    if (!token || !executionID.trim()) return;
    setBusyKey("execution");
    try {
      const nextExecution = await getMCPExecution(token, executionID.trim());
      setExecution(nextExecution);
      try {
        const [installation, executionTools] = await Promise.all([
          getMCPInstallation(token, nextExecution.installation_id),
          listMCPInstallationTools(token, nextExecution.installation_id),
        ]);
        setExecutionInstallation(installation);
        setExecutionTool(
          executionTools.find((tool) => tool.id === nextExecution.tool_id) ?? null,
        );
      } catch {
        setExecutionInstallation(null);
        setExecutionTool(null);
      }
      setNotice("");
    } catch (error) {
      setExecution(null);
      setExecutionInstallation(null);
      setExecutionTool(null);
      setNotice(errorMessage(error));
    } finally {
      setBusyKey("");
    }
  }, [executionID, token]);

  const revisionLabel = useMemo(() => {
    const revision = selectedInstallation?.latest_revision;
    if (!revision) return "Revision pending";
    return `Revision ${revision.revision} · ${revision.scan_status}`;
  }, [selectedInstallation]);

  if (!currentOrganization) {
    return (
      <SafeAreaView style={styles.emptyState}>
        <Text style={styles.emptyTitle}>未选择工作区</Text>
        <Text style={styles.mutedText}>请先在组织页面选择一个工作区。</Text>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
        <View style={styles.header}>
          <View style={styles.headerCopy}>
            <Text style={styles.eyebrow}>{currentOrganization.name}</Text>
            <Text style={styles.title}>Agent 工具平台</Text>
          </View>
          <View style={styles.headerActions}>
            <ActionButton
              label="Trace / 审批"
              compact
              onPress={() => navigation.navigate("AgentDemo")}
            />
            {busyKey === "refresh" ? (
              <ActivityIndicator color="#2563eb" />
            ) : (
              <ActionButton label="刷新" compact onPress={() => void refresh()} />
            )}
          </View>
        </View>

        <SegmentedControl
          value={activeTab}
          onChange={setActiveTab}
          options={[
            { value: "installations", label: "安装" },
            { value: "skills", label: "Skills" },
            { value: "execution", label: "执行" },
          ]}
        />

        {notice ? <Text style={styles.notice}>{notice}</Text> : null}

        {activeTab === "installations" ? (
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
                  onPress={() => void handleCreateInstallation()}
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
                      onPress={() => void selectInstallation(installation)}
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
                      onPress={confirmDisableInstallation}
                    />
                  </View>

                  <View style={styles.divider} />
                  <Text style={styles.formTitle}>Secret 字段</Text>
                  <Text style={styles.sectionMeta}>
                    已保存的值不会回显
                  </Text>
                  {secretDrafts.map((secret, index) => (
                    <View key={index} style={styles.secretRow}>
                      <View style={styles.secretField}>
                        <Field
                          label="名称"
                          value={secret.key}
                          editable={canManageSelectedInstallation}
                          autoCapitalize="characters"
                          error={secretErrors[String(index)]}
                          onChangeText={(key) => updateSecretRow(index, { key })}
                          placeholder="API_TOKEN"
                        />
                      </View>
                      <View style={styles.secretField}>
                        <Field
                          label="值"
                          value={secret.value}
                          editable={canManageSelectedInstallation}
                          secureTextEntry
                          autoCapitalize="none"
                          onChangeText={(value) => updateSecretRow(index, { value })}
                          placeholder="Secret value"
                        />
                      </View>
                      {secretDrafts.length > 1 ? (
                        <ActionButton
                          label="移除"
                          compact
                          danger
                          disabled={!canManageSelectedInstallation}
                          onPress={() =>
                            setSecretDrafts((current) =>
                              current.filter((_, itemIndex) => itemIndex !== index),
                            )
                          }
                        />
                      ) : null}
                    </View>
                  ))}
                  {secretErrors.form ? <Text style={styles.errorText}>{secretErrors.form}</Text> : null}
                  <View style={styles.actionRow}>
                    <ActionButton
                      label="添加字段"
                      compact
                      disabled={!canManageSelectedInstallation}
                      onPress={() =>
                        setSecretDrafts((current) => [...current, { key: "", value: "" }])
                      }
                    />
                    <ActionButton
                      label={busyKey === "save-secrets" ? "保存中..." : "安全保存"}
                      compact
                      disabled={busyKey !== "" || !canManageSelectedInstallation}
                      onPress={() => void handleSaveSecrets()}
                    />
                  </View>

                  <View style={styles.divider} />
                  <Text style={styles.formTitle}>工具目录</Text>
                  {tools.length === 0 ? (
                    <Text style={styles.emptyInline}>验证后将显示工具</Text>
                  ) : (
                    tools.map((tool) => (
                      <Pressable
                        key={tool.id}
                        onPress={() => toggleTool(tool.id)}
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
        ) : null}

        {activeTab === "skills" ? (
          <>
            <View style={styles.sectionHeader}>
              <View>
                <Text style={styles.sectionTitle}>Skills</Text>
                <Text style={styles.sectionMeta}>{skills.length} 个可见 Skill</Text>
              </View>
            </View>
            <View style={styles.formBand}>
              <Text style={styles.formTitle}>创建 Skill</Text>
              <Text style={styles.fieldLabel}>作用域</Text>
              <SegmentedControl<MCPScope>
                value={skillScope}
                onChange={setSkillScope}
                options={[
                  { value: "personal", label: "个人" },
                  { value: "organization", label: "组织发布" },
                ]}
              />
              {!isAdmin && skillScope === "organization" ? (
                <Text style={styles.warningText}>组织 Skill 需要管理员权限</Text>
              ) : null}
              <Field label="名称" value={skillName} onChangeText={setSkillName} />
              <Field
                label="描述"
                value={skillDescription}
                onChangeText={setSkillDescription}
              />
              <Field
                label="Agent 指令"
                value={skillInstructions}
                multiline
                onChangeText={setSkillInstructions}
              />
              <View style={styles.bindingSummary}>
                <Text style={styles.fieldLabel}>绑定工具</Text>
                <Text style={styles.sectionMeta}>{selectedToolIds.length} 个已选</Text>
              </View>
              {tools.length === 0 ? (
                <Text style={styles.emptyInline}>在“安装”页选择安装及工具</Text>
              ) : (
                tools.map((tool) => (
                  <Pressable
                    key={tool.id}
                    onPress={() => toggleTool(tool.id)}
                    style={[
                      styles.toolRow,
                      selectedToolIds.includes(tool.id) && styles.toolRowSelected,
                    ]}
                  >
                    <View style={styles.flexCopy}>
                      <Text style={styles.toolName}>{tool.name}</Text>
                      <Text style={styles.itemMeta}>{tool.risk} · revision #{tool.revision_id}</Text>
                    </View>
                    <Text style={styles.selectionMark}>
                      {selectedToolIds.includes(tool.id) ? "已选" : "选择"}
                    </Text>
                  </Pressable>
                ))
              )}
              {!canBindSelectedToolsToSkill && selectedToolIds.length > 0 ? (
                <Text style={styles.warningText}>
                  组织 Skill 只能绑定已发布到组织的工具
                </Text>
              ) : null}
              <ActionButton
                label={busyKey === "create-skill" ? "创建中..." : "创建 Skill"}
                disabled={
                  busyKey !== "" ||
                  selectedToolIds.length === 0 ||
                  !canBindSelectedToolsToSkill ||
                  (!isAdmin && skillScope === "organization")
                }
                onPress={() => void handleCreateSkill()}
              />
            </View>

            {skills.map((skill) => (
              <View key={skill.id} style={styles.itemCard}>
                <View style={styles.rowBetween}>
                  <View style={styles.flexCopy}>
                    <Text style={styles.itemTitle}>{skill.name}</Text>
                    <Text style={styles.itemMeta}>
                      {skill.scope} · version {skill.version}
                    </Text>
                  </View>
                  <StatusBadge status={skill.status} />
                </View>
                {skill.description ? (
                  <Text style={styles.toolDescription}>{skill.description}</Text>
                ) : null}
                {skill.published_at ? (
                  <Text style={styles.itemMeta}>
                    组织发布于 {new Date(skill.published_at).toLocaleString()}
                  </Text>
                ) : null}
                <Text style={styles.skillInstructions} numberOfLines={4}>
                  {skill.instructions}
                </Text>
                {!canManageScopedResource(skill.scope, isAdmin) ? (
                  <Text style={styles.warningText}>组织 Skill 仅 owner 或 admin 可修改</Text>
                ) : null}
                <View style={styles.actionRow}>
                  <ActionButton
                    compact
                    label={`更新绑定 (${selectedToolIds.length})`}
                    disabled={
                      busyKey !== "" ||
                      !canManageScopedResource(skill.scope, isAdmin) ||
                      !selectedInstallation ||
                      !canBindInstallationToSkill(
                        skill.scope,
                        selectedInstallation.scope,
                      )
                    }
                    onPress={() => void updateSkillBinding(skill)}
                  />
                  <ActionButton
                    compact
                    label={skill.status === "active" ? "停用" : "启用"}
                    disabled={
                      busyKey !== "" ||
                      !canManageScopedResource(skill.scope, isAdmin)
                    }
                    onPress={() => void toggleSkillStatus(skill)}
                  />
                  <ActionButton
                    compact
                    danger
                    label="删除"
                    disabled={
                      busyKey !== "" ||
                      !canManageScopedResource(skill.scope, isAdmin)
                    }
                    onPress={() => void removeSkill(skill)}
                  />
                </View>
              </View>
            ))}
          </>
        ) : null}

        {activeTab === "execution" ? (
          <>
            <View style={styles.sectionHeader}>
              <View>
                <Text style={styles.sectionTitle}>执行结果</Text>
                <Text style={styles.sectionMeta}>按 execution ID 查询</Text>
              </View>
            </View>
            <View style={styles.executionSearch}>
              <View style={styles.flexCopy}>
                <Field
                  label="Execution ID"
                  value={executionID}
                  autoCapitalize="none"
                  onChangeText={setExecutionID}
                  placeholder="mcp:..."
                />
              </View>
              <ActionButton
                label={busyKey === "execution" ? "查询中..." : "查询"}
                compact
                disabled={busyKey !== "" || !executionID.trim()}
                onPress={() => void handleFetchExecution()}
              />
            </View>
            {execution ? (
              <View style={styles.executionDetail}>
                <View style={styles.rowBetween}>
                  <View style={styles.flexCopy}>
                    <Text style={styles.detailTitle}>{execution.execution_id}</Text>
                    <Text style={styles.itemMeta}>
                      {executionTool?.name ?? `Tool #${execution.tool_id}`} · revision #{execution.revision_id} · attempts {execution.attempts}
                    </Text>
                  </View>
                  <StatusBadge status={execution.status} />
                </View>
                {execution.error_message ? (
                  <Text style={styles.warningText}>{execution.error_message}</Text>
                ) : null}
                <View style={styles.detailGrid}>
                  <Text style={styles.detailKey}>来源</Text>
                  <Text style={styles.detailValue}>
                    {executionInstallation?.source_type.toUpperCase() ??
                      `Installation #${execution.installation_id}`}
                  </Text>
                  <Text style={styles.detailKey}>风险</Text>
                  <Text style={styles.detailValue}>
                    {executionTool?.risk ?? "unknown"}
                  </Text>
                </View>
                <Text style={styles.approvalPolicy}>
                  {executionTool
                    ? toolExecutionPolicy(executionTool.risk)
                    : "工具风险和审批策略由 Go 网关判定"}
                </Text>
                <Text style={styles.payloadHeading}>输入</Text>
                <Text style={styles.payload}>{formatPlatformJSON(execution.input)}</Text>
                <View style={styles.untrustedHeader}>
                  <Text style={styles.payloadHeading}>工具输出</Text>
                  <Text style={styles.untrustedLabel}>不可信数据</Text>
                </View>
                <Text style={styles.payload}>{formatPlatformJSON(execution.output)}</Text>
              </View>
            ) : null}
          </>
        ) : null}
      </ScrollView>
    </SafeAreaView>
  );
};

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#f8fafc" },
  content: { padding: 16, paddingBottom: 48, width: "100%", maxWidth: 1120, alignSelf: "center" },
  header: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: 16 },
  headerCopy: { flex: 1 },
  headerActions: { flexDirection: "row", alignItems: "center", gap: 8 },
  eyebrow: { fontSize: 12, color: "#64748b", textTransform: "uppercase" },
  title: { fontSize: 26, lineHeight: 34, fontWeight: "700", color: "#0f172a" },
  segmentedControl: { flexDirection: "row", borderWidth: 1, borderColor: "#cbd5e1", borderRadius: 8, padding: 3, backgroundColor: "#fff", marginBottom: 16 },
  segment: { flex: 1, minHeight: 38, paddingHorizontal: 8, alignItems: "center", justifyContent: "center", borderRadius: 6 },
  segmentSelected: { backgroundColor: "#0f172a" },
  segmentText: { fontSize: 13, color: "#475569", fontWeight: "600" },
  segmentTextSelected: { color: "#fff" },
  notice: { borderLeftWidth: 3, borderLeftColor: "#2563eb", backgroundColor: "#eff6ff", color: "#1e40af", padding: 10, marginBottom: 14, lineHeight: 19 },
  sectionHeader: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginTop: 4, marginBottom: 12 },
  sectionTitle: { fontSize: 20, fontWeight: "700", color: "#0f172a" },
  sectionMeta: { fontSize: 12, lineHeight: 18, color: "#64748b" },
  formBand: { backgroundColor: "#fff", borderTopWidth: 1, borderBottomWidth: 1, borderColor: "#e2e8f0", paddingVertical: 16, paddingHorizontal: 14, marginHorizontal: -16, marginBottom: 16 },
  formTitle: { fontSize: 16, fontWeight: "700", color: "#1e293b", marginBottom: 10 },
  field: { marginBottom: 12 },
  fieldLabel: { fontSize: 13, fontWeight: "600", color: "#334155", marginBottom: 6 },
  input: { minHeight: 44, borderWidth: 1, borderColor: "#cbd5e1", borderRadius: 8, paddingHorizontal: 12, paddingVertical: 10, backgroundColor: "#fff", color: "#0f172a", fontSize: 15 },
  multilineInput: { minHeight: 88, textAlignVertical: "top" },
  inputError: { borderColor: "#dc2626" },
  errorText: { color: "#b91c1c", fontSize: 12, marginTop: 4 },
  warningText: { color: "#9a3412", backgroundColor: "#fff7ed", padding: 9, marginBottom: 10, lineHeight: 18 },
  actionButton: { minHeight: 42, borderRadius: 8, paddingHorizontal: 16, paddingVertical: 10, backgroundColor: "#2563eb", alignItems: "center", justifyContent: "center" },
  actionButtonCompact: { minHeight: 36, paddingHorizontal: 12, paddingVertical: 7 },
  actionButtonDanger: { backgroundColor: "#fff", borderWidth: 1, borderColor: "#ef4444" },
  actionButtonDisabled: { opacity: 0.45 },
  actionButtonPressed: { opacity: 0.78 },
  actionButtonText: { color: "#fff", fontSize: 13, fontWeight: "700" },
  actionButtonDangerText: { color: "#b91c1c" },
  installationLayout: { gap: 14 },
  listPane: { gap: 8 },
  detailPane: { backgroundColor: "#fff", borderWidth: 1, borderColor: "#e2e8f0", borderRadius: 8, padding: 14 },
  itemCard: { backgroundColor: "#fff", borderWidth: 1, borderColor: "#e2e8f0", borderRadius: 8, padding: 14, marginBottom: 8 },
  itemCardSelected: { borderColor: "#2563eb", backgroundColor: "#eff6ff" },
  rowBetween: { flexDirection: "row", alignItems: "flex-start", justifyContent: "space-between", gap: 10 },
  flexCopy: { flex: 1, minWidth: 0 },
  itemTitle: { fontSize: 15, lineHeight: 21, fontWeight: "700", color: "#0f172a" },
  itemMeta: { fontSize: 12, lineHeight: 18, color: "#64748b", marginTop: 3 },
  detailTitle: { fontSize: 18, lineHeight: 24, fontWeight: "700", color: "#0f172a" },
  badge: { alignSelf: "flex-start", borderRadius: 6, paddingHorizontal: 7, paddingVertical: 4 },
  badgeText: { fontSize: 11, fontWeight: "700" },
  detailGrid: { display: "flex", flexDirection: "row", flexWrap: "wrap", marginTop: 14, marginBottom: 10 },
  detailKey: { width: "22%", fontSize: 12, color: "#64748b", marginBottom: 7 },
  detailValue: { width: "28%", fontSize: 12, fontWeight: "600", color: "#1e293b", marginBottom: 7 },
  monospace: { fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }), fontSize: 11, lineHeight: 17, color: "#475569", backgroundColor: "#f8fafc", padding: 8, marginBottom: 8 },
  actionRow: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 10 },
  divider: { height: 1, backgroundColor: "#e2e8f0", marginVertical: 18 },
  secretRow: { gap: 8, marginBottom: 4 },
  secretField: { flex: 1 },
  toolRow: { flexDirection: "row", alignItems: "flex-start", gap: 10, borderTopWidth: 1, borderTopColor: "#e2e8f0", paddingVertical: 12, paddingHorizontal: 8 },
  toolRowSelected: { backgroundColor: "#eff6ff" },
  toolName: { fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }), fontSize: 12, lineHeight: 18, fontWeight: "600", color: "#0f172a" },
  toolDescription: { fontSize: 13, lineHeight: 19, color: "#475569", marginTop: 6 },
  approvalReason: { fontSize: 10, color: "#64748b", marginTop: 5, textAlign: "right" },
  approvalPolicy: { fontSize: 12, lineHeight: 18, color: "#9a3412", backgroundColor: "#fff7ed", padding: 9, marginTop: 10 },
  bindingSummary: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  selectionMark: { fontSize: 12, fontWeight: "700", color: "#2563eb" },
  skillInstructions: { fontSize: 12, lineHeight: 18, color: "#334155", backgroundColor: "#f8fafc", padding: 9, marginTop: 9 },
  executionSearch: { flexDirection: "row", gap: 10, alignItems: "center" },
  executionDetail: { backgroundColor: "#fff", borderWidth: 1, borderColor: "#e2e8f0", borderRadius: 8, padding: 14 },
  payloadHeading: { fontSize: 13, fontWeight: "700", color: "#334155", marginTop: 14, marginBottom: 6 },
  payload: { fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }), fontSize: 11, lineHeight: 17, color: "#334155", backgroundColor: "#f8fafc", padding: 10 },
  untrustedHeader: { flexDirection: "row", alignItems: "flex-end", justifyContent: "space-between" },
  untrustedLabel: { fontSize: 11, fontWeight: "700", color: "#9a3412", backgroundColor: "#ffedd5", paddingHorizontal: 7, paddingVertical: 3, marginBottom: 6 },
  emptyInline: { color: "#64748b", fontSize: 13, lineHeight: 20, paddingVertical: 12 },
  emptyState: { flex: 1, justifyContent: "center", alignItems: "center", padding: 24, backgroundColor: "#f8fafc" },
  emptyTitle: { fontSize: 20, fontWeight: "700", color: "#0f172a", marginBottom: 8 },
  mutedText: { color: "#64748b", textAlign: "center" },
});

export default MCPPlatformScreen;
