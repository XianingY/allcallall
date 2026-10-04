import { memo, useCallback } from "react";
import { FlatList, Text, TouchableOpacity } from "react-native";
import MessageRow from "./MessageRow";
import MessageComposer from "./MessageComposer";
import { styles } from "./styles";
import type { MessagePaneProps } from "./types";
import type { MessageRecord } from "../../api/collaboration";

const messageKeyExtractor = (item: MessageRecord) => String(item.id);

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
  const renderMessage = useCallback(
    ({ item }: { item: MessageRecord }) => (
      <MessageRow
        item={item}
        currentUserId={currentUserId}
        onOpenTranscript={onOpenTranscript}
        onLongPress={onLongPressMessage}
        onDownloadAttachment={onDownloadAttachment}
      />
    ),
    [
      currentUserId,
      onOpenTranscript,
      onLongPressMessage,
      onDownloadAttachment,
    ],
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
      keyExtractor={messageKeyExtractor}
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
        <MessageComposer
          sending={sending}
          workflowLoading={workflowLoading}
          pendingAttachments={pendingAttachments}
          uploadingAttachment={uploadingAttachment}
          onSend={onSend}
          onAskAgent={onAskAgent}
          onPickAttachment={onPickAttachment}
          onRemovePendingAttachment={onRemovePendingAttachment}
        />
      }
    />
  );
};

export default memo(MessagePane);
