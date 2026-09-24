import { Pressable, ScrollView, Text, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import TextField from "../../components/TextField";
import {
  CitationList,
  SignalList,
  StatusPill,
  ToolCallList,
  TraceTimeline,
} from "./components";
import { workflowPresets } from "../agentDemoUtils";
import { styles } from "./styles";
import type { AgentLabController } from "./types";

  export const RunTab = ({ controller }: { controller: AgentLabController }) => {
  const activeRun = controller.activeRun;
  const activeWorkflow = controller.activeWorkflow;
  return (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.panel}>
        <View style={styles.panelHeader}>
          <View>
            <Text style={styles.sectionTitle}>
              {controller.selectedConversation?.title || "Select a conversation"}
            </Text>
            <Text style={styles.contextLine}>
              Messages {controller.messages.length} · Notes {controller.notes.length}
            </Text>
          </View>
          <View style={styles.headerActions}>
            <PrimaryButton
              title="Start ReAct"
              onPress={controller.handleStartReactRun}
              disabled={controller.busy || !controller.selectedConversationId}
              style={styles.startButton}
            />
            <PrimaryButton
              title="Start Workflow"
              onPress={controller.handleStartWorkflow}
              disabled={controller.busy || !controller.selectedConversationId}
              style={styles.workflowButton}
            />
          </View>
        </View>
        <TextField
          label="Goal"
          value={controller.goal}
          onChangeText={controller.setGoal}
          multiline
          style={styles.goalInput}
        />
        <Text style={styles.subsectionLabel}>Workflow preset</Text>
        <View style={styles.segmentRow}>
          {workflowPresets.map((preset) => {
            const selected = controller.workflowPreset === preset.key;
            return (
              <Pressable
                key={preset.key}
                style={[styles.segmentButton, selected && styles.segmentActive]}
                onPress={() => controller.setWorkflowPreset(preset.key)}
              >
                <Text
                  style={[
                    styles.segmentText,
                    selected && styles.segmentTextActive,
                  ]}
                >
                  {preset.label}
                </Text>
              </Pressable>
            );
          })}
        </View>
      </View>

      {activeRun ? (
        <View style={styles.panel}>
          <View style={styles.panelHeader}>
            <View>
              <Text style={styles.sectionTitle}>
                ReAct Run #{activeRun.run.id}
              </Text>
              <Text style={styles.contextLine}>
                {activeRun.run.source || "agent"} · {activeRun.run.goal}
              </Text>
            </View>
            <View style={styles.headerActions}>
              <StatusPill status={activeRun.run.status} />
              <Pressable
                style={styles.inlineButton}
                onPress={() => void controller.refreshActiveRun(activeRun.run.id)}
              >
                <Text style={styles.inlineButtonText}>Refresh</Text>
              </Pressable>
            </View>
          </View>
          {activeRun.run.summary ? (
            <Text style={styles.answerText}>{activeRun.run.summary}</Text>
          ) : null}
          {activeRun.run.next_step ? (
            <Text style={styles.nextStep}>
              Next step: {activeRun.run.next_step}
            </Text>
          ) : null}
          <SignalList title="Action Items" items={activeRun.run.action_items} />
          <SignalList title="Risk Flags" items={activeRun.run.risk_flags} />
          <TraceTimeline trace={activeRun.trace} events={controller.runEvents} />
          <ToolCallList toolCalls={activeRun.tool_calls} />
          <CitationList
            citations={activeRun.citations}
            onPress={controller.handleCitationPress}
          />
        </View>
      ) : null}

      {activeWorkflow ? (
        <View style={styles.panel}>
          <View style={styles.panelHeader}>
            <View>
              <Text style={styles.sectionTitle}>
                Workflow #{activeWorkflow.workflow.id}
              </Text>
              <Text style={styles.contextLine}>
                {activeWorkflow.workflow.preset || "custom"} ·{" "}
                {activeWorkflow.workflow.goal}
              </Text>
            </View>
            <StatusPill status={activeWorkflow.workflow.status} />
          </View>
          {activeWorkflow.workflow.summary ? (
            <Text style={styles.answerText}>
              {activeWorkflow.workflow.summary}
            </Text>
          ) : null}
          {activeWorkflow.workflow.next_step ? (
            <Text style={styles.nextStep}>
              Next step: {activeWorkflow.workflow.next_step}
            </Text>
          ) : null}
          <SignalList
            title="Action Items"
            items={activeWorkflow.workflow.action_items}
          />
          <SignalList
            title="Risk Flags"
            items={activeWorkflow.workflow.risk_flags}
          />
          {controller.pendingWorkflowApprovals.length > 0 ? (
            <Text style={styles.warningText}>
              Waiting for {controller.pendingWorkflowApprovals.length} tool approval
              {controller.pendingWorkflowApprovals.length > 1 ? "s" : ""}.
            </Text>
          ) : null}
          <CitationList
            citations={activeWorkflow.citations}
            onPress={controller.handleCitationPress}
          />
        </View>
      ) : null}

      <View style={styles.grid}>
        <View style={[styles.panel, styles.contextPanel]}>
          <Text style={styles.sectionTitle}>Messages</Text>
          {controller.messages.slice(-6).map((message) => (
            <View key={message.id} style={styles.contextItem}>
              <Text style={styles.contextItemTitle}>
                {message.sender_display_name ||
                  message.sender_email ||
                  message.type}
              </Text>
              <Text style={styles.contextItemBody}>
                {message.body || message.type}
              </Text>
            </View>
          ))}
          {controller.messages.length === 0 ? (
            <Text style={styles.emptyText}>No messages.</Text>
          ) : null}
        </View>
        <View style={[styles.panel, styles.contextPanel]}>
          <Text style={styles.sectionTitle}>Notes</Text>
          {controller.notes.slice(0, 6).map((note) => (
            <View key={note.id} style={styles.contextItem}>
              <Text style={styles.contextItemTitle}>
                {note.author_display_name || note.author_email}
              </Text>
              <Text style={styles.contextItemBody}>{note.body}</Text>
            </View>
          ))}
          {controller.notes.length === 0 ? (
            <Text style={styles.emptyText}>No notes.</Text>
          ) : null}
        </View>
      </View>
    </ScrollView>
  );
}