import React, { useMemo } from "react";
import { Text, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import { styles } from "./styles";
import type { MessageRowProps } from "./types";

// 提取为 memo 行组件，避免列表整体重渲染；日期字符串在 memo 内 useMemo 预计算，
// 不在渲染体里反复 new Date().toLocaleString()。
const MessageRow = React.memo<MessageRowProps>(
  ({ item, currentUserId, onOpenTranscript }) => {
    const isMine = item.sender_id === currentUserId;
    const isSystem = item.type === "system";
    const timeLabel = useMemo(
      () => new Date(item.created_at).toLocaleString(),
      [item.created_at],
    );
    const eventType = item.metadata?.event_type;
    const recordingId =
      eventType === "meeting.transcription.ready" &&
      typeof item.metadata?.recording_id === "number"
        ? (item.metadata.recording_id as number)
        : undefined;

    return (
      <View
        style={[
          styles.messageBubble,
          isSystem ? styles.systemBubble : isMine ? styles.mine : styles.theirs,
        ]}
      >
        <Text style={styles.sender}>
          {item.sender_display_name || item.sender_email}
        </Text>
        <Text style={styles.body}>{item.body || item.type}</Text>
        {eventType ? (
          <Text style={styles.systemMeta}>{String(eventType)}</Text>
        ) : null}
        {recordingId !== undefined ? (
          <PrimaryButton
            title="查看会议转写"
            onPress={() => onOpenTranscript(recordingId)}
            style={styles.systemAction}
          />
        ) : null}
        <Text style={styles.time}>{timeLabel}</Text>
      </View>
    );
  },
);

export default MessageRow;
