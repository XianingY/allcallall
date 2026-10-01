import { formatBytes, formatShortDateTime } from "@allcallall/shared";
import React, { useMemo } from "react";
import { Pressable, Text, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import type { AttachmentRecord } from "../../api/collaboration";
import { styles } from "./styles";
import type { MessageRowProps } from "./types";

const AttachmentChip = ({
  attachment,
  onDownload,
}: {
  attachment: AttachmentRecord;
  onDownload: (attachment: AttachmentRecord) => void;
}) => (
  <Pressable style={styles.attachmentChip} onPress={() => onDownload(attachment)}>
    <View style={styles.attachmentChipBody}>
      <Text style={styles.attachmentName} numberOfLines={1}>
        {attachment.file_name}
      </Text>
      <Text style={styles.attachmentMeta}>
        {formatBytes(attachment.file_size)}
      </Text>
    </View>
    <Text style={styles.attachmentAction}>下载</Text>
  </Pressable>
);

// 提取为 memo 行组件，避免列表整体重渲染；日期字符串在 memo 内 useMemo 预计算，
// 不在渲染体里反复 new Date().toLocaleString()。
const MessageRow = React.memo<MessageRowProps>(
  ({ item, currentUserId, onOpenTranscript, onLongPress, onDownloadAttachment }) => {
    const isMine = item.sender_id === currentUserId;
    const isSystem = item.type === "system";
    // Shared with web: a fixed shape, so the same message shows the same
    // timestamp on both clients instead of following the device locale.
    const timeLabel = useMemo(
      () => formatShortDateTime(item.created_at),
      [item.created_at],
    );
    const eventType = item.metadata?.event_type;
    const recordingId =
      eventType === "meeting.transcription.ready" &&
      typeof item.metadata?.recording_id === "number"
        ? (item.metadata.recording_id as number)
        : undefined;

    // 撤回/删除的墓碑态：信封保留（谁、何时），正文与附件不再展示。
    if (item.recalled_at) {
      return (
        <View style={styles.tombstoneBubble}>
          <Text style={styles.tombstoneText}>
            {isMine
              ? "你撤回了一条消息"
              : `${item.sender_display_name || item.sender_email} 撤回了一条消息`}
          </Text>
          <Text style={styles.time}>{timeLabel}</Text>
        </View>
      );
    }
    if (item.deleted_at) {
      return (
        <View style={styles.tombstoneBubble}>
          <Text style={styles.tombstoneText}>消息已删除</Text>
          <Text style={styles.time}>{timeLabel}</Text>
        </View>
      );
    }

    return (
      <Pressable
        onLongPress={() => onLongPress(item)}
        delayLongPress={350}
        style={[
          styles.messageBubble,
          isSystem ? styles.systemBubble : isMine ? styles.mine : styles.theirs,
        ]}
      >
        <Text style={styles.sender}>
          {item.sender_display_name || item.sender_email}
        </Text>
        <Text style={styles.body}>{item.body || item.type}</Text>
        {item.edited_at ? <Text style={styles.editedTag}>已编辑</Text> : null}
        {item.attachments?.length ? (
          <View style={styles.attachmentList}>
            {item.attachments.map((attachment) => (
              <AttachmentChip
                key={attachment.id}
                attachment={attachment}
                onDownload={onDownloadAttachment}
              />
            ))}
          </View>
        ) : null}
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
      </Pressable>
    );
  },
);

export default MessageRow;
