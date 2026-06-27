import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  FlatList,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import {
  fetchRecordingTranscript,
  retryRecordingTranscription,
  type MeetingTranscriptSegmentRecord,
  type RecordingTranscriptionRecord,
} from "../api/collaboration";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import type { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "RecordingTranscript">;

const formatTimestamp = (milliseconds: number) => {
  const totalSeconds = Math.max(0, Math.floor(milliseconds / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`
    : `${minutes}:${String(seconds).padStart(2, "0")}`;
};

const statusLabel = (status?: string) => {
  switch (status) {
    case "pending":
      return "等待转写";
    case "processing":
      return "转写处理中";
    case "ready":
      return "转写完成";
    case "failed":
      return "转写失败";
    case "skipped":
      return "未生成转写";
    default:
      return "尚未请求转写";
  }
};

const RecordingTranscriptScreen: React.FC<Props> = ({ route }) => {
  const { recordingId, segmentId, startMs } = route.params;
  const { token } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const listRef = useRef<FlatList<MeetingTranscriptSegmentRecord>>(null);
  const [transcription, setTranscription] = useState<RecordingTranscriptionRecord | null>(null);
  const [segments, setSegments] = useState<MeetingTranscriptSegmentRecord[]>([]);
  const [nextAfterId, setNextAfterId] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const [highlightedId, setHighlightedId] = useState<number | null>(segmentId ?? null);

  const canRetry =
    transcription?.status === "failed" &&
    (currentOrganization?.role === "owner" || currentOrganization?.role === "admin");

  const loadInitial = useCallback(async () => {
    if (!token) return;
    try {
      setLoading(true);
      const page = await fetchRecordingTranscript(token, recordingId, { limit: 100 });
      setTranscription(page.transcription ?? null);
      setSegments(page.segments);
      setNextAfterId(page.next_after_id ?? null);
    } catch (error) {
      console.error("[RecordingTranscriptScreen] Failed to load transcript:", error);
      Alert.alert("加载失败", "无法加载会议转写。");
    } finally {
      setLoading(false);
    }
  }, [recordingId, token]);

  useEffect(() => {
    void loadInitial();
  }, [loadInitial]);

  useEffect(() => {
    if (segments.length === 0) return;
    const targetIndex = segmentId
      ? segments.findIndex((item) => item.id === segmentId)
      : typeof startMs === "number"
        ? segments.findIndex((item) => item.start_ms <= startMs && item.end_ms >= startMs)
        : -1;
    if (targetIndex >= 0) {
      setHighlightedId(segments[targetIndex].id);
      requestAnimationFrame(() => {
        listRef.current?.scrollToIndex({ index: targetIndex, animated: true, viewPosition: 0.3 });
      });
    }
  }, [segmentId, segments, startMs]);

  const loadMore = useCallback(async () => {
    if (!token || !nextAfterId || loadingMore) return;
    try {
      setLoadingMore(true);
      const page = await fetchRecordingTranscript(token, recordingId, {
        afterId: nextAfterId,
        limit: 100,
      });
      setTranscription(page.transcription ?? null);
      setSegments((current) => [...current, ...page.segments]);
      setNextAfterId(page.next_after_id ?? null);
    } catch (error) {
      console.error("[RecordingTranscriptScreen] Failed to load more transcript:", error);
    } finally {
      setLoadingMore(false);
    }
  }, [loadingMore, nextAfterId, recordingId, token]);

  const handleRetry = useCallback(async () => {
    if (!token || !canRetry) return;
    try {
      setRetrying(true);
      const next = await retryRecordingTranscription(token, recordingId);
      setTranscription(next);
      setSegments([]);
      setNextAfterId(null);
    } catch (error) {
      console.error("[RecordingTranscriptScreen] Failed to retry transcript:", error);
      Alert.alert("重试失败", "无法重新提交转写任务。");
    } finally {
      setRetrying(false);
    }
  }, [canRetry, recordingId, token]);

  const header = useMemo(
    () => (
      <View style={styles.header}>
        <Text style={styles.heading}>会议录音 #{recordingId}</Text>
        <View style={styles.statusRow}>
          <Text style={styles.status}>{statusLabel(transcription?.status)}</Text>
          {transcription?.provider ? (
            <Text style={styles.provider}>{transcription.provider}</Text>
          ) : null}
        </View>
        <Text style={styles.meta}>
          {transcription?.segment_count ?? 0} segments
          {transcription?.completed_at
            ? ` · ${new Date(transcription.completed_at).toLocaleString()}`
            : ""}
        </Text>
        {transcription?.error_message ? (
          <Text style={styles.error}>{transcription.error_message}</Text>
        ) : null}
        {canRetry ? (
          <PrimaryButton
            title={retrying ? "正在重新提交" : "重新转写"}
            onPress={() => void handleRetry()}
            disabled={retrying}
            style={styles.retryButton}
          />
        ) : null}
      </View>
    ),
    [canRetry, handleRetry, recordingId, retrying, transcription],
  );

  if (loading && segments.length === 0) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" />
      </View>
    );
  }

  return (
    <FlatList
      ref={listRef}
      style={styles.container}
      contentContainerStyle={styles.content}
      data={segments}
      keyExtractor={(item) => String(item.id)}
      ListHeaderComponent={header}
      refreshing={loading}
      onRefresh={() => void loadInitial()}
      onEndReached={() => void loadMore()}
      onEndReachedThreshold={0.35}
      onScrollToIndexFailed={({ index }) => {
        setTimeout(() => listRef.current?.scrollToIndex({ index, animated: false }), 150);
      }}
      renderItem={({ item }) => (
        <Pressable
          style={[styles.segment, highlightedId === item.id && styles.segmentHighlighted]}
          onPress={() => setHighlightedId(item.id)}
        >
          <View style={styles.segmentHeader}>
            <Text style={styles.timestamp}>{formatTimestamp(item.start_ms)}</Text>
            <Text style={styles.speaker}>
              {item.speaker_user_id ? `Speaker ${item.speaker_user_id}` : item.track_key || "Speaker"}
            </Text>
            {item.language ? <Text style={styles.language}>{item.language}</Text> : null}
          </View>
          <Text style={styles.segmentText}>{item.text}</Text>
        </Pressable>
      )}
      ListEmptyComponent={
        <View style={styles.empty}>
          <Text style={styles.emptyText}>
            {transcription?.status === "processing" || transcription?.status === "pending"
              ? "录音转写正在处理。"
              : transcription?.status === "failed"
                ? "本次转写失败，请查看上方错误信息。"
                : "当前录音还没有转写内容。"}
          </Text>
        </View>
      }
      ListFooterComponent={loadingMore ? <ActivityIndicator style={styles.footer} /> : null}
    />
  );
};

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#f8fafc" },
  content: { padding: 16, paddingBottom: 40 },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  header: { marginBottom: 16 },
  heading: { fontSize: 22, fontWeight: "700", color: "#0f172a" },
  statusRow: { flexDirection: "row", alignItems: "center", gap: 8, marginTop: 10 },
  status: { color: "#0f766e", fontWeight: "700" },
  provider: { color: "#475569" },
  meta: { marginTop: 6, color: "#64748b" },
  error: { marginTop: 10, color: "#b91c1c", lineHeight: 20 },
  retryButton: { marginTop: 12, backgroundColor: "#b45309" },
  segment: {
    backgroundColor: "#fff",
    borderWidth: 1,
    borderColor: "#e2e8f0",
    borderRadius: 8,
    padding: 14,
    marginBottom: 10,
  },
  segmentHighlighted: { borderColor: "#0f766e", backgroundColor: "#f0fdfa" },
  segmentHeader: { flexDirection: "row", alignItems: "center", gap: 8, flexWrap: "wrap" },
  timestamp: { color: "#0f766e", fontWeight: "700", fontVariant: ["tabular-nums"] },
  speaker: { color: "#334155", fontWeight: "600" },
  language: { color: "#64748b" },
  segmentText: { marginTop: 8, color: "#0f172a", lineHeight: 22 },
  empty: { paddingVertical: 48, alignItems: "center" },
  emptyText: { color: "#64748b", textAlign: "center" },
  footer: { marginVertical: 16 },
});

export default RecordingTranscriptScreen;
