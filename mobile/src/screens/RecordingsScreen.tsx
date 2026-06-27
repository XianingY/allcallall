import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { buildRecordingDownloadRequest, listRecordings, type RecordingRecord } from "../api/collaboration";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import fileDownloadAdapter from "../platform/fileDownload";
import type { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "Recordings">;

const RecordingsScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const [items, setItems] = useState<RecordingRecord[]>([]);
  const [loading, setLoading] = useState(false);

  const loadData = useCallback(async () => {
    if (!token) {
      setItems([]);
      return;
    }
    try {
      setLoading(true);
      const data = await listRecordings(token);
      setItems(data);
    } catch (error) {
      console.error("[RecordingsScreen] Failed to load recordings:", error);
      Alert.alert("加载失败", "无法加载录音存档列表。");
    } finally {
      setLoading(false);
    }
  }, [token]);

  const handleDownload = useCallback(async (recordingId: number, fileId: number, fileName: string) => {
    if (!token) {
      return;
    }
    try {
      const request = buildRecordingDownloadRequest(token, recordingId, fileId);
      const result = await fileDownloadAdapter.download(request, fileName || `recording-${fileId}`);
      try {
        await fileDownloadAdapter.open(result);
      } catch {
        Alert.alert("下载完成", `文件已保存到 ${result.location}`);
      }
    } catch (error) {
      console.error("[RecordingsScreen] Failed to download recording:", error);
      Alert.alert("下载失败", "无法下载录音文件。");
    }
  }, [token]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  return (
    <View style={styles.container}>
      <Text style={styles.heading}>录音存档</Text>
      <FlatList
        data={items}
        keyExtractor={(item) => String(item.session.id)}
        refreshing={loading}
        onRefresh={() => void loadData()}
        renderItem={({ item }) => (
          <View style={styles.card}>
            <Text style={styles.title}>会议 #{item.session.room_id}</Text>
            <Text style={styles.meta}>状态 {item.session.status}</Text>
            <Text style={styles.meta}>文件数 {item.files.length}</Text>
            <Text style={styles.meta}>
              转写 {item.transcription?.status ?? "not_requested"}
              {item.transcription?.segment_count
                ? ` · ${item.transcription.segment_count} segments`
                : ""}
            </Text>
            {item.transcription ? (
              <PrimaryButton
                title="查看会议转写"
                onPress={() =>
                  navigation.navigate("RecordingTranscript", {
                    recordingId: item.session.id,
                  })
                }
                style={styles.transcriptButton}
              />
            ) : null}
            {item.files.map((file) => (
              <View key={file.id} style={styles.fileRow}>
                <Text style={styles.fileType}>{file.recording_kind} · {file.content_type}</Text>
                <Text style={styles.filePath}>{file.file_name}</Text>
                <Text style={styles.meta}>大小 {file.file_size_bytes} bytes · 时长 {file.duration_seconds}s</Text>
                <PrimaryButton
                  title="下载"
                  onPress={() => void handleDownload(item.session.id, file.id, file.file_name)}
                  style={styles.downloadButton}
                />
              </View>
            ))}
          </View>
        )}
        ListEmptyComponent={
          <View style={styles.empty}>
            <Text style={styles.emptyText}>当前工作区还没有录音存档。</Text>
          </View>
        }
      />
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16
  },
  heading: {
    fontSize: 22,
    fontWeight: "700",
    color: "#0f172a",
    marginBottom: 12
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "#e2e8f0"
  },
  title: {
    fontSize: 17,
    fontWeight: "600",
    color: "#0f172a"
  },
  meta: {
    color: "#64748b",
    marginTop: 4
  },
  fileRow: {
    marginTop: 10,
    paddingTop: 10,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0"
  },
  fileType: {
    color: "#0f172a",
    fontWeight: "600"
  },
  filePath: {
    marginTop: 4,
    color: "#475569"
  },
  downloadButton: {
    marginTop: 10
  },
  transcriptButton: {
    marginTop: 10,
    backgroundColor: "#0f766e"
  },
  empty: {
    alignItems: "center",
    paddingTop: 48
  },
  emptyText: {
    color: "#64748b"
  }
});

export default RecordingsScreen;
