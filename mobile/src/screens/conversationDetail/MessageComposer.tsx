import { memo, useCallback, useState } from "react";
import { Text, TouchableOpacity, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import TextField from "../../components/TextField";
import { styles } from "./styles";
import type { MessageComposerProps } from "./types";

const MessageComposer = ({
  sending,
  workflowLoading,
  pendingAttachments,
  uploadingAttachment,
  onSend,
  onAskAgent,
  onPickAttachment,
  onRemovePendingAttachment,
}: MessageComposerProps) => {
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

  return (
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
  );
};

export default memo(MessageComposer);
