import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Platform } from "react-native";

import {
  createAgentSkill,
  createMCPInstallation,
  deleteAgentSkill,
  disableMCPInstallation,
  getMCPExecution,
  getMCPInstallation,
  listAgentSkills,
  listMCPInstallations,
  listMCPInstallationTools,
  putMCPInstallationSecrets,
  updateAgentSkill,
  type AgentSkill,
  type MCPExecution,
  type MCPInstallation,
  type MCPTool,
} from "../../api/mcpPlatform";
import { useAuthContext } from "../../context/AuthContext";
import { useOrganization } from "../../context/OrganizationContext";
import {
  canBindInstallationToSkill,
  canManageScopedResource,
  validateInstallationDraft,
  validateSecretDrafts,
  type MCPInstallationDraft,
  type MCPScope,
  type SecretDraft,
} from "../mcpPlatformUtils";
import { EMPTY_INSTALLATION, errorMessage } from "./constants";
import type { PlatformTab } from "./types";

/**
 * MCPPlatformScreen 的全部数据与写操作收敛到这一个 hook 中。
 * 编排器只负责布局与分段切换，把返回值透传给三个面板。
 */
export function useMCPPlatform() {
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

  return {
    currentOrganization,
    token,
    activeTab,
    setActiveTab,
    installations,
    skills,
    selectedInstallation,
    tools,
    selectedToolIds,
    installationDraft,
    setInstallationDraft,
    installationErrors,
    setInstallationErrors,
    showInstaller,
    setShowInstaller,
    secretDrafts,
    setSecretDrafts,
    secretErrors,
    setSecretErrors,
    skillName,
    setSkillName,
    skillDescription,
    setSkillDescription,
    skillInstructions,
    setSkillInstructions,
    skillScope,
    setSkillScope,
    executionID,
    setExecutionID,
    execution,
    executionInstallation,
    executionTool,
    busyKey,
    notice,
    isAdmin,
    canManageSelectedInstallation,
    canBindSelectedToolsToSkill,
    revisionLabel,
    refresh,
    selectInstallation,
    runInstallationAction,
    handleCreateInstallation,
    confirmDisableInstallation,
    updateSecretRow,
    handleSaveSecrets,
    toggleTool,
    handleCreateSkill,
    toggleSkillStatus,
    updateSkillBinding,
    removeSkill,
    handleFetchExecution,
  };
}
