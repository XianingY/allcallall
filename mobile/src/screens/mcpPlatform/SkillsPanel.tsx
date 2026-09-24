import React from "react";
import { Pressable, Text, View } from "react-native";

import type {
  AgentSkill,
  MCPInstallation,
  MCPTool
} from "../../api/mcpPlatform";
import {
  canBindInstallationToSkill,
  canManageScopedResource,
  type MCPScope
} from "../mcpPlatformUtils";

import { ActionButton, Field, SegmentedControl, StatusBadge } from "./primitives";
import { styles } from "./styles";

interface SkillsPanelProps {
  skills: AgentSkill[];
  isAdmin: boolean;
  skillScope: MCPScope;
  setSkillScope: React.Dispatch<React.SetStateAction<MCPScope>>;
  skillName: string;
  setSkillName: React.Dispatch<React.SetStateAction<string>>;
  skillDescription: string;
  setSkillDescription: React.Dispatch<React.SetStateAction<string>>;
  skillInstructions: string;
  setSkillInstructions: React.Dispatch<React.SetStateAction<string>>;
  selectedToolIds: number[];
  tools: MCPTool[];
  onToggleTool: (toolID: number) => void;
  canBindSelectedToolsToSkill: boolean;
  busyKey: string;
  onCreateSkill: () => void;
  selectedInstallation: MCPInstallation | null;
  onUpdateSkillBinding: (skill: AgentSkill) => void;
  onToggleSkillStatus: (skill: AgentSkill) => void;
  onRemoveSkill: (skill: AgentSkill) => void;
}

export const SkillsPanel: React.FC<SkillsPanelProps> = ({
  skills,
  isAdmin,
  skillScope,
  setSkillScope,
  skillName,
  setSkillName,
  skillDescription,
  setSkillDescription,
  skillInstructions,
  setSkillInstructions,
  selectedToolIds,
  tools,
  onToggleTool,
  canBindSelectedToolsToSkill,
  busyKey,
  onCreateSkill,
  selectedInstallation,
  onUpdateSkillBinding,
  onToggleSkillStatus,
  onRemoveSkill
}) => (
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
                    onPress={() => onToggleTool(tool.id)}
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
                onPress={() => void onCreateSkill()}
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
                    onPress={() => void onUpdateSkillBinding(skill)}
                  />
                  <ActionButton
                    compact
                    label={skill.status === "active" ? "停用" : "启用"}
                    disabled={
                      busyKey !== "" ||
                      !canManageScopedResource(skill.scope, isAdmin)
                    }
                    onPress={() => void onToggleSkillStatus(skill)}
                  />
                  <ActionButton
                    compact
                    danger
                    label="删除"
                    disabled={
                      busyKey !== "" ||
                      !canManageScopedResource(skill.scope, isAdmin)
                    }
                    onPress={() => void onRemoveSkill(skill)}
                  />
                </View>
              </View>
            ))}
          </>
);
