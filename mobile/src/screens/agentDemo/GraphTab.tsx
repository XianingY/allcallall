import { ScrollView, Text, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import { StatusPill } from "./components";
import { compact, formatTime, jsonSummary, parseJSON } from "../agentDemoUtils";
import { styles } from "./styles";
import type { AgentLabController } from "./types";

  export const GraphTab = ({ controller }: { controller: AgentLabController }) => {
  const activeWorkflow = controller.activeWorkflow;
  return (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.panel}>
        <View style={styles.panelHeader}>
          <Text style={styles.sectionTitle}>Task Graph</Text>
          <PrimaryButton
            title="Process"
            onPress={controller.handleProcessWorkflow}
            disabled={controller.busy || !activeWorkflow}
            style={styles.processButton}
          />
        </View>
        {activeWorkflow ? (
          <Text style={styles.contextLine}>
            {activeWorkflow.workflow.workflow_version || "agent_lab_v1"} ·{" "}
            {activeWorkflow.workflow.prompt_version || "-"} ·{" "}
            {activeWorkflow.workflow.tool_schema_version || "-"}
          </Text>
        ) : null}
        {controller.orderedTasks.map((task, index) => (
          <View key={task.id} style={styles.taskRow}>
            <View style={styles.taskIndex}>
              <Text style={styles.taskIndexText}>{index + 1}</Text>
            </View>
            <View style={styles.taskBody}>
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle}>{task.name}</Text>
                <StatusPill status={task.status} />
              </View>
              <Text style={styles.rowMeta}>
                {task.role} · depends{" "}
                {compact(task.depends_on_json || "[]", 80)}
              </Text>
              {task.error_message ? (
                <Text style={styles.errorText}>{task.error_message}</Text>
              ) : null}
              {task.input_json ? (
                <Text style={styles.messageBody}>
                  Input: {jsonSummary(task.input_json, 220)}
                </Text>
              ) : null}
              {task.output_json ? (
                <Text style={styles.messageBody}>
                  Output: {jsonSummary(task.output_json, 320)}
                </Text>
              ) : null}
            </View>
          </View>
        ))}
        {!activeWorkflow ? (
          <Text style={styles.emptyText}>No workflow selected.</Text>
        ) : null}
      </View>

      <View style={styles.panel}>
        <Text style={styles.sectionTitle}>Agent Messages</Text>
        {(activeWorkflow?.messages ?? []).map((message) => (
          <View key={message.id} style={styles.messageBox}>
            <Text style={styles.rowTitle}>
              {message.from_role} → {message.to_role}
            </Text>
            <Text style={styles.rowMeta}>
              {message.message_type} · {message.correlation_id}
            </Text>
            <Text style={styles.messageBody}>
              {compact(
                JSON.stringify(
                  parseJSON(message.content_json) ?? message.content_json,
                ),
                460,
              )}
            </Text>
          </View>
        ))}
        {activeWorkflow && activeWorkflow.messages.length === 0 ? (
          <Text style={styles.emptyText}>No agent messages.</Text>
        ) : null}
      </View>

      <View style={styles.grid}>
        <View style={[styles.panel, styles.contextPanel]}>
          <Text style={styles.sectionTitle}>History</Text>
          {(activeWorkflow?.history ?? []).map((event) => (
            <View key={event.id} style={styles.messageBox}>
              <Text style={styles.rowTitle}>{event.event_type}</Text>
              <Text style={styles.rowMeta}>
                {event.ref_type || "workflow"} · {formatTime(event.created_at)}
              </Text>
              {event.attributes_json ? (
                <Text style={styles.messageBody}>
                  {compact(event.attributes_json, 260)}
                </Text>
              ) : null}
            </View>
          ))}
          {activeWorkflow && activeWorkflow.history.length === 0 ? (
            <Text style={styles.emptyText}>No history events.</Text>
          ) : null}
        </View>
        <View style={[styles.panel, styles.contextPanel]}>
          <Text style={styles.sectionTitle}>Signals & Timers</Text>
          {(activeWorkflow?.signals ?? []).map((signal) => (
            <View key={`signal-${signal.id}`} style={styles.messageBox}>
              <Text style={styles.rowTitle}>{signal.signal_name}</Text>
              <Text style={styles.rowMeta}>
                {signal.status} · {formatTime(signal.created_at)}
              </Text>
            </View>
          ))}
          {(activeWorkflow?.timers ?? []).map((timer) => (
            <View key={`timer-${timer.id}`} style={styles.messageBox}>
              <Text style={styles.rowTitle}>{timer.timer_name}</Text>
              <Text style={styles.rowMeta}>
                {timer.status} · due {formatTime(timer.fire_at)}
              </Text>
            </View>
          ))}
          {activeWorkflow &&
          activeWorkflow.signals.length === 0 &&
          activeWorkflow.timers.length === 0 ? (
            <Text style={styles.emptyText}>No signals or timers.</Text>
          ) : null}
        </View>
      </View>
    </ScrollView>
  );
}