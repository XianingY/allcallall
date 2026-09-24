import { Linking, Modal, Pressable, ScrollView, Text, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import { styles } from "./styles";
import type {
  CitationPreviewModalProps,
  KnowledgePreviewModalProps,
  WorkflowDebugModalProps,
} from "./types";

export const KnowledgePreviewModal = ({
  knowledgePreview,
  onClose,
}: KnowledgePreviewModalProps) => (
  <Modal
    visible={knowledgePreview !== null}
    transparent
    animationType="slide"
    onRequestClose={onClose}
  >
    <View style={styles.modalBackdrop}>
      <View style={styles.modalCard}>
        <Text style={styles.modalTitle}>
          {knowledgePreview?.source.title || "Knowledge Preview"}
        </Text>
        {knowledgePreview ? (
          <ScrollView style={styles.modalScroll}>
            <Text style={styles.infoMeta}>
              {knowledgePreview.source.kind} · versions{" "}
              {knowledgePreview.versions.length} · chunks{" "}
              {knowledgePreview.chunks.length}
            </Text>
            {knowledgePreview.source.uri ? (
              <Pressable
                style={styles.linkRow}
                onPress={() =>
                  void Linking.openURL(knowledgePreview.source.uri || "")
                }
              >
                <Text style={styles.linkText}>Open origin URL</Text>
              </Pressable>
            ) : null}
            {knowledgePreview.chunks.slice(0, 8).map((chunk) => (
              <View key={chunk.id} style={styles.modalSection}>
                <Text style={styles.citationTitle}>
                  Chunk {chunk.chunk_index}
                </Text>
                <Text style={styles.citationMeta}>
                  {chunk.index_status} · offsets {chunk.start_offset}-
                  {chunk.end_offset}
                </Text>
                <Text style={styles.citationSnippet}>{chunk.snippet}</Text>
              </View>
            ))}
          </ScrollView>
        ) : null}
        <PrimaryButton
          title="Close"
          onPress={onClose}
          style={styles.modalButton}
        />
      </View>
    </View>
  </Modal>
);

export const CitationPreviewModal = ({
  citationPreview,
  onClose,
}: CitationPreviewModalProps) => (
  <Modal
    visible={citationPreview !== null}
    transparent
    animationType="fade"
    onRequestClose={onClose}
  >
    <View style={styles.modalBackdrop}>
      <View style={styles.modalCard}>
        <Text style={styles.modalTitle}>
          {citationPreview?.source_title ||
            citationPreview?.title ||
            "Citation"}
        </Text>
        {citationPreview ? (
          <>
            <Text style={styles.infoMeta}>
              {citationPreview.source_type} ·{" "}
              {citationPreview.retrieval_mode || "context"} · score{" "}
              {citationPreview.score}
            </Text>
            <Text style={styles.citationSnippet}>
              {citationPreview.snippet}
            </Text>
          </>
        ) : null}
        <PrimaryButton
          title="Close"
          onPress={onClose}
          style={styles.modalButton}
        />
      </View>
    </View>
  </Modal>
);

export const WorkflowDebugModal = ({
  visible,
  activeWorkflow,
  orderedTasks,
  workflowLoading,
  token,
  onClose,
  onProcess,
}: WorkflowDebugModalProps) => (
  <Modal
    visible={visible}
    transparent
    animationType="slide"
    onRequestClose={onClose}
  >
    <View style={styles.modalBackdrop}>
      <View style={styles.debugDrawer}>
        <Text style={styles.modalTitle}>Workflow Debug</Text>
        <Text style={styles.infoMeta}>
          {activeWorkflow?.workflow.workflow_version || "-"} ·{" "}
          {activeWorkflow?.workflow.status || "no workflow"}
        </Text>
        <ScrollView style={styles.modalScroll}>
          <Text style={styles.debugHeader}>Tasks</Text>
          {orderedTasks.map((task) => (
            <View key={task.id} style={styles.modalSection}>
              <Text style={styles.citationTitle}>
                {task.name} · {task.status}
              </Text>
              <Text style={styles.citationMeta}>
                {task.role} · attempts {task.attempts}
              </Text>
              {task.error_message ? (
                <Text style={styles.errorText}>{task.error_message}</Text>
              ) : null}
            </View>
          ))}
          <Text style={styles.debugHeader}>History</Text>
          {(activeWorkflow?.history ?? []).map((event) => (
            <View key={event.id} style={styles.modalSection}>
              <Text style={styles.citationTitle}>{event.event_type}</Text>
              <Text style={styles.citationMeta}>
                {event.ref_type || "workflow"} ·{" "}
                {new Date(event.created_at).toLocaleString()}
              </Text>
            </View>
          ))}
          <Text style={styles.debugHeader}>Signals & Timers</Text>
          {(activeWorkflow?.signals ?? []).map((signal) => (
            <View key={`signal-${signal.id}`} style={styles.modalSection}>
              <Text style={styles.citationTitle}>
                {signal.signal_name} · {signal.status}
              </Text>
            </View>
          ))}
          {(activeWorkflow?.timers ?? []).map((timer) => (
            <View key={`timer-${timer.id}`} style={styles.modalSection}>
              <Text style={styles.citationTitle}>
                {timer.timer_name} · {timer.status}
              </Text>
              <Text style={styles.citationMeta}>
                due {new Date(timer.fire_at).toLocaleString()}
              </Text>
            </View>
          ))}
          <Text style={styles.debugHeader}>Agent Messages</Text>
          {(activeWorkflow?.messages ?? []).map((message) => (
            <View key={message.id} style={styles.modalSection}>
              <Text style={styles.citationTitle}>
                {message.from_role} → {message.to_role}
              </Text>
              <Text style={styles.citationMeta}>
                {message.message_type}
              </Text>
              <Text style={styles.citationSnippet}>
                {message.content_json}
              </Text>
            </View>
          ))}
        </ScrollView>
        <View style={styles.buttonRow}>
          <PrimaryButton
            title="Close"
            onPress={onClose}
            style={styles.button}
          />
          <PrimaryButton
            title="Process"
            onPress={onProcess}
            disabled={!activeWorkflow || workflowLoading || !token}
            style={styles.buttonSecondary}
          />
        </View>
      </View>
    </View>
  </Modal>
);
