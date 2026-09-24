import React from "react";
import {
  ActivityIndicator,
  SafeAreaView,
  ScrollView,
  Text,
  View
} from "react-native";

import {
  ActionButton,
  ExecutionPanel,
  InstallationsPanel,
  SegmentedControl,
  SkillsPanel,
  styles
} from "./mcpPlatform";
import type { MCPPlatformScreenProps as Props } from "./mcpPlatform";
import { useMCPPlatform } from "./mcpPlatform/useMCPPlatform";

const MCPPlatformScreen: React.FC<Props> = ({ navigation }) => {
  const {
    currentOrganization,
    activeTab,
    setActiveTab,
    installations,
    selectedInstallation,
    selectInstallation,
    showInstaller,
    setShowInstaller,
    installationDraft,
    setInstallationDraft,
    installationErrors,
    handleCreateInstallation,
    isAdmin,
    busyKey,
    revisionLabel,
    canManageSelectedInstallation,
    runInstallationAction,
    confirmDisableInstallation,
    secretDrafts,
    setSecretDrafts,
    secretErrors,
    updateSecretRow,
    handleSaveSecrets,
    tools,
    selectedToolIds,
    toggleTool,
    skills,
    skillScope,
    setSkillScope,
    skillName,
    setSkillName,
    skillDescription,
    setSkillDescription,
    skillInstructions,
    setSkillInstructions,
    canBindSelectedToolsToSkill,
    handleCreateSkill,
    updateSkillBinding,
    toggleSkillStatus,
    removeSkill,
    token,
    executionID,
    setExecutionID,
    handleFetchExecution,
    execution,
    executionTool,
    executionInstallation,
    notice,
    refresh
  } = useMCPPlatform();

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
          <InstallationsPanel
            installations={installations}
            selectedInstallation={selectedInstallation}
            onSelectInstallation={selectInstallation}
            showInstaller={showInstaller}
            setShowInstaller={setShowInstaller}
            installationDraft={installationDraft}
            setInstallationDraft={setInstallationDraft}
            installationErrors={installationErrors}
            onCreateInstallation={handleCreateInstallation}
            isAdmin={isAdmin}
            busyKey={busyKey}
            revisionLabel={revisionLabel}
            canManageSelectedInstallation={canManageSelectedInstallation}
            runInstallationAction={runInstallationAction}
            token={token}
            onDisableInstallation={confirmDisableInstallation}
            secretDrafts={secretDrafts}
            setSecretDrafts={setSecretDrafts}
            secretErrors={secretErrors}
            onUpdateSecretRow={updateSecretRow}
            onSaveSecrets={handleSaveSecrets}
            tools={tools}
            selectedToolIds={selectedToolIds}
            onToggleTool={toggleTool}
          />
        ) : null}

        {activeTab === "skills" ? (
          <SkillsPanel
            skills={skills}
            isAdmin={isAdmin}
            skillScope={skillScope}
            setSkillScope={setSkillScope}
            skillName={skillName}
            setSkillName={setSkillName}
            skillDescription={skillDescription}
            setSkillDescription={setSkillDescription}
            skillInstructions={skillInstructions}
            setSkillInstructions={setSkillInstructions}
            selectedToolIds={selectedToolIds}
            tools={tools}
            onToggleTool={toggleTool}
            canBindSelectedToolsToSkill={canBindSelectedToolsToSkill}
            busyKey={busyKey}
            onCreateSkill={handleCreateSkill}
            selectedInstallation={selectedInstallation}
            onUpdateSkillBinding={updateSkillBinding}
            onToggleSkillStatus={toggleSkillStatus}
            onRemoveSkill={removeSkill}
          />
        ) : null}

        {activeTab === "execution" ? (
          <ExecutionPanel
            executionID={executionID}
            setExecutionID={setExecutionID}
            busyKey={busyKey}
            onFetchExecution={handleFetchExecution}
            execution={execution}
            executionTool={executionTool}
            executionInstallation={executionInstallation}
          />
        ) : null}
      </ScrollView>
    </SafeAreaView>
  );
};

export default MCPPlatformScreen;
