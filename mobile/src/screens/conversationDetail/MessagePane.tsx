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
  draft,
  workflowLoading,
  currentUserId,
  onRefresh,
  onLoadMorePrev,
  onDraftChange,
  onSend,
  onAskAgent,
  onOpenTranscript,
}: MessagePaneProps) => {
  const renderMessage = ({ item }: { item: MessageRecord }) => (
    <MessageRow
      item={item}
      currentUserId={currentUserId}
      onOpenTranscript={onOpenTranscript}
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
            <View style={styles.buttonRow}>
              <PrimaryButton
                title="发送消息"
                onPress={onSend}
                disabled={!draft.trim()}
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
