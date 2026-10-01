import { FlatList, Text, TouchableOpacity, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import TextField from "../../components/TextField";
import MessageRow from "./MessageRow";
import { styles } from "./styles";
import type { MessagePaneProps } from "./types";
import type { MessageRecord } from "../../api/collaboration";

type Props = MessagePaneProps & {
  sending: boolean;
};

const MessagePane = ({
  messages,
  loading,
  hasMorePrev,
  loadingMorePrev,
  draft,
  sending,
  workflowLoading,
  currentUserId,
  pendingAttachments,
  uploadingAttachment,
  onRefresh,
  onLoadMorePrev,
  onDraftChange,
  onSend,
  onAskAgent,
  onOpenTranscript,
  onLongPressMessage,
  onDownloadAttachment,
  onPickAttachment,
  onRemovePendingAttachment,
}: Props) => {
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
              onChangeText={onDraftChange}
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
                onPress={onSend}
                disabled={!draft.trim() || sending}
                style={styles.button}
              />
              <PrimaryButton
                title="Run Agent"
                onPress={onAskAgent}
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
