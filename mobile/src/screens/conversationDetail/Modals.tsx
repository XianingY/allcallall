import { Linking, Modal, Pressable, ScrollView, Text, View } from "react-native";
import { formatShortDateTime } from "@allcallall/shared";
import PrimaryButton from "../../components/PrimaryButton";
import TextField from "../../components/TextField";
import { styles } from "./styles";
import type {
  CitationPreviewModalProps,
  EditMessageModalProps,
  KnowledgePreviewModalProps,
  MessageActionMenuModalProps,
  WorkflowDebugModalProps,
} from "./types";

/** 长按消息弹出的操作菜单：编辑/撤回仅限本人，删除额外允许组织管理员。 */
export const MessageActionMenuModal = ({
  visible,
  message,
  currentUserId,
  canDeleteAny,
  onClose,
  onEdit,
  onRecall,
  onDelete,
}: MessageActionMenuModalProps) => {
  const isMine = message != null && message.sender_id === currentUserId;
  return (
    <Modal
      visible={visible}
      transparent
      animationType="fade"
      onRequestClose={onClose}
    >
      <Pressable style={styles.modalBackdrop} onPress={onClose}>
        <Pressable style={styles.modalCard} onPress={() => undefined}>
          <Text style={styles.modalTitle}>消息操作</Text>
          {isMine && message?.type === "text" && !message.recalled_at && !message.deleted_at ? (
            <Pressable
              style={styles.actionMenuOption}
              onPress={() => message && onEdit(message)}
            >
              <Text style={styles.actionMenuOptionText}>编辑</Text>
            </Pressable>
          ) : null}
          {isMine && !message?.recalled_at && !message?.deleted_at ? (
            <Pressable
              style={[styles.actionMenuOption, { marginTop: 8 }]}
              onPress={() => message && onRecall(message)}
            >
              <Text style={styles.actionMenuOptionText}>撤回</Text>
            </Pressable>
          ) : null}
          {isMine || canDeleteAny ? (
            <Pressable
              style={[styles.actionMenuOption, styles.actionMenuOptionDanger, { marginTop: 8 }]}
              onPress={() => message && onDelete(message)}
            >
              <Text
                style={[
                  styles.actionMenuOptionText,
                  styles.actionMenuOptionDangerText,
                ]}
              >
                删除
              </Text>
            </Pressable>
          ) : null}
          <PrimaryButton
            title="取消"
            onPress={onClose}
            style={styles.modalButton}
          />
        </Pressable>
      </Pressable>
    </Modal>
  );
};

/** 编辑消息正文（仅发送者本人）。 */
export const EditMessageModal = ({
  visible,
  message,
  draft,
  saving,
  onDraftChange,
  onClose,
  onSave,
}: EditMessageModalProps) => (
  <Modal
    visible={visible && message !== null}
    transparent
    animationType="fade"
    onRequestClose={onClose}
  >
    <View style={styles.modalBackdrop}>
      <View style={styles.modalCard}>
        <Text style={styles.modalTitle}>编辑消息</Text>
        <TextField
          value={draft}
          onChangeText={onDraftChange}
          placeholder="修改消息内容"
        />
        <View style={styles.buttonRow}>
          <PrimaryButton
            title="取消"
            onPress={onClose}
            style={styles.button}
          />
          <PrimaryButton
            title={saving ? "保存中…" : "保存"}
            onPress={onSave}
            disabled={!draft.trim() || saving}
            style={styles.buttonSecondary}
          />
        </View>
      </View>
    </View>
  </Modal>
);

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
                {formatShortDateTime(event.created_at)}
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
                due {formatShortDateTime(timer.fire_at)}
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
