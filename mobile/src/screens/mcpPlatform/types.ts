import type { NativeStackScreenProps } from "@react-navigation/native-stack";

import type {
  AgentSkill,
  MCPExecution,
  MCPInstallation,
  MCPTool
} from "../../api/mcpPlatform";
import type { RootStackParamList } from "../../navigation/AppNavigator";
import type {
  MCPInstallationDraft,
  MCPScope,
  SecretDraft
} from "../mcpPlatformUtils";

export type MCPPlatformScreenProps = NativeStackScreenProps<
  RootStackParamList,
  "MCPPlatform"
>;

export type PlatformTab = "installations" | "skills" | "execution";

/** Skill 创建表单的可变字段，用于把多个 setter 收敛成一个 onChange。 */
export interface SkillDraftPatch {
  scope?: MCPScope;
  name?: string;
  description?: string;
  instructions?: string;
}

/** 安装创建表单（新增安装面板）所需的全部状态与回调。 */
export interface InstallationFormController {
  show: boolean;
  draft: MCPInstallationDraft;
  errors: Record<string, string>;
  onToggle: () => void;
  onChange: (patch: Partial<MCPInstallationDraft>) => void;
  onSubmit: () => void;
}

/** Secret 编辑区所需的状态与回调。 */
export interface SecretEditorController {
  drafts: SecretDraft[];
  errors: Record<string, string>;
  onUpdateRow: (index: number, patch: Partial<SecretDraft>) => void;
  onAddRow: () => void;
  onRemoveRow: (index: number) => void;
  onSave: () => void;
}

/** 工具目录的选中状态，安装页与 Skill 页共用这一份选择。 */
export interface ToolSelectionController {
  tools: MCPTool[];
  selectedToolIds: number[];
  onToggle: (toolID: number) => void;
}

/** 单个安装上可执行的写操作。runAction 保持与原编排器一致的签名。 */
export interface InstallationActionController {
  token: string | null;
  runAction: (
    key: string,
    action: () => Promise<MCPInstallation>,
    success: string
  ) => void;
  onDisable: () => void;
}

/** Skill 创建表单所需的状态与回调。 */
export interface SkillFormController {
  scope: MCPScope;
  name: string;
  description: string;
  instructions: string;
  onChange: (patch: SkillDraftPatch) => void;
  onSubmit: () => void;
}

/** 执行结果查询条所需的状态与回调。 */
export interface ExecutionQueryController {
  executionID: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
}

export interface ExecutionResult {
  execution: MCPExecution | null;
  installation: MCPInstallation | null;
  tool: MCPTool | null;
}

export type SkillMutateHandler = (skill: AgentSkill) => void;
