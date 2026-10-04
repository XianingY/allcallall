import { useCallback, useState } from "react";
import { FlatList, Text, TouchableOpacity, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import TextField from "../../components/TextField";
import MessageRow from "./MessageRow";
import { styles } from "./styles";
import type { MessagePaneProps } from "./types";
import type { MessageRecord } from "../../api/collaboration";

const MessagePane = ({
  messages,
  loading,
  hasMorePrev,
  loadingMorePrev,
  sending,
  workflowLoading,
  currentUserId,
  pendingAttachments,
  uploadingAttachment,
  onRefresh,
  onLoadMorePrev,
  onSend,
  onAskAgent,
  onOpenTranscript,
  onLongPressMessage,
  onDownloadAttachment,
  onPickAttachment,
  onRemovePendingAttachment,
}: MessagePaneProps) => {
  const [draft, setDraft] = useState("");
  const send = useCallback(async () => {
    if (!draft.trim() || sending) return;
    const sent = await onSend(draft);
    if (sent) {
      setDraft("");
    }
  }, [draft, onSend, sending]);

  const askAgent = useCallback(async () => {
    if (workflowLoading) return;
    const sent = await onAskAgent(draft);
    if (sent) {
      setDraft("");
    }
  }, [draft, onAskAgent, workflowLoading]);

  const renderMessage = ({ item }: { item: MessageRecord }) => (
    <MessageRow
      item={item}
      currentUserId={currentUserId}
      onOpenTranscript={onOpenTranscript}
      onLongPress={onLongPressMessage}
      onDownloadAttachment={onDownloadAttachment}
    />
  );

  return (
    <FlatList
      // Bounds how much is kept mounted and rendered per batch. The
      // defaults (21 / 10 / 10) are tuned for short lists; these lists
      // grow with the workspace.
      windowSize={7}
      initialNumToRender={12}
      maxToRenderPerBatch={12}
      data={messages}
      keyExtractor={(item) => String(item.id)}
      refreshing={loading}
      onRefresh={onRefresh}
      contentContainerStyle={styles.listContent}
      renderItem={renderMessage}
      ListHeaderComponent={
        hasMorePrev ? (
          <TouchableOpacity
            style={styles.loadEarlier}
            onPress={() => void onLoadMorePrev()}
            disabled={loadingMorePrev}
          >
            <Text style={styles.loadEarlierText}>
              {loadingMorePrev ? "加载中…" : "加载更早的消息"}
            </Text>
          </TouchableOpacity>
        ) : null
      }
      ListFooterComponent={
        <View>
          <View style={styles.composer}>
            <TextField
              value={draft}
              onChangeText={setDraft}
              placeholder="输入线程消息，或输入自定义 Agent goal"
            />
            {pendingAttachments.length ? (
              <View style={styles.pendingAttachmentRow}>
                {pendingAttachments.map((attachment) => (
                  <TouchableOpacity
                    key={attachment.id}
                    style={styles.pendingAttachmentChip}
                    onPress={() => onRemovePendingAttachment(attachment.id)}
                  >
                    <Text style={styles.pendingAttachmentName} numberOfLines={1}>
                      {attachment.file_name}
                    </Text>
                    <Text style={styles.pendingAttachmentRemove}>✕</Text>
                  </TouchableOpacity>
                ))}
              </View>
            ) : null}
            <View style={styles.buttonRow}>
              <PrimaryButton
                title={uploadingAttachment ? "上传中…" : "＋附件"}
                onPress={onPickAttachment}
                disabled={uploadingAttachment}
                style={styles.buttonSecondary}
              />
              <PrimaryButton
                title={sending ? "发送中…" : "发送消息"}
                onPress={() => void send()}
                disabled={!draft.trim() || sending}
                style={styles.button}
              />
              <PrimaryButton
                title="Run Agent"
                onPress={() => void askAgent()}
                disabled={workflowLoading}
                style={styles.buttonSecondary}
              />
            </View>
          </View>
        </View>
      }
    />
  );
};

export default MessagePane;
