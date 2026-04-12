import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, Linking, StyleSheet, Text, View } from "react-native";
import RNFS from "react-native-fs";

import { buildRecordingDownloadRequest, listRecordings, type RecordingRecord } from "../api/collaboration";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";

const RecordingsScreen: React.FC = () => {
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

  const handleDownload = useCallback(async (recordingId: number, fileId: number, objectKey: string) => {
    if (!token) {
      return;
    }
    try {
      const request = buildRecordingDownloadRequest(token, recordingId, fileId);
      const destination = `${RNFS.DocumentDirectoryPath}/${objectKey.split("/").pop() ?? `recording-${fileId}`}`;
      const result = await RNFS.downloadFile({
        ...request,
        toFile: destination,
        background: true,
        discretionary: true
      }).promise;
      if (result.statusCode < 200 || result.statusCode >= 300) {
        throw new Error(`download failed with status ${result.statusCode}`);
      }
      try {
        await Linking.openURL(`file://${destination}`);
      } catch {
        Alert.alert("下载完成", `文件已保存到 ${destination}`);
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
            {item.files.map((file) => (
              <View key={file.id} style={styles.fileRow}>
                <Text style={styles.fileType}>{file.content_type}</Text>
                <Text style={styles.filePath}>{file.object_key}</Text>
                <PrimaryButton
                  title="下载"
                  onPress={() => void handleDownload(item.session.id, file.id, file.object_key)}
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
  empty: {
    alignItems: "center",
    paddingTop: 48
  },
  emptyText: {
    color: "#64748b"
  }
});

export default RecordingsScreen;
