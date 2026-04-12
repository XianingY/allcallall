import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import {
  fetchRoomState,
  joinRoom,
  leaveRoom,
  startRoomRecording,
  stopRoomRecording,
  type RoomRecord,
  type RecordingRecord
} from "../api/collaboration";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "RoomDetail">;

const RoomDetailScreen: React.FC<Props> = ({ route, navigation }) => {
  const { token } = useAuthContext();
  const [room, setRoom] = useState<RoomRecord>(route.params.room);
  const [recording, setRecording] = useState<RecordingRecord | null>(null);
  const [loading, setLoading] = useState(false);

  const loadData = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      setLoading(true);
      const next = await fetchRoomState(token, room.room.id);
      setRoom(next);
    } catch (error) {
      console.error("[RoomDetailScreen] Failed to load room:", error);
      Alert.alert("加载失败", "无法加载会议详情。");
    } finally {
      setLoading(false);
    }
  }, [room.room.id, token]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useEffect(() => {
    const timer = setInterval(() => {
      void loadData();
    }, 5000);
    return () => clearInterval(timer);
  }, [loadData]);

  const handleJoin = async () => {
    if (!token) {
      return;
    }
    try {
      setRoom(await joinRoom(token, room.room.id));
      await loadData();
    } catch (error) {
      console.error("[RoomDetailScreen] Failed to join room:", error);
      Alert.alert("加入失败", "无法加入当前会议房间。");
    }
  };

  const handleLeave = async () => {
    if (!token) {
      return;
    }
    try {
      setRoom(await leaveRoom(token, room.room.id));
      await loadData();
    } catch (error) {
      console.error("[RoomDetailScreen] Failed to leave room:", error);
      Alert.alert("离开失败", "无法离开当前会议房间。");
    }
  };

  const handleStartRecording = async () => {
    if (!token) {
      return;
    }
    try {
      const next = await startRoomRecording(token, room.room.id);
      setRecording(next);
      await loadData();
    } catch (error) {
      console.error("[RoomDetailScreen] Failed to start recording:", error);
      Alert.alert("开启失败", "当前策略不允许录音，或无法启动录音。");
    }
  };

  const handleStopRecording = async () => {
    if (!token) {
      return;
    }
    try {
      const next = await stopRoomRecording(token, room.room.id);
      setRecording(next);
      await loadData();
      Alert.alert("录音已完成", "会议录音产物已生成。");
    } catch (error) {
      console.error("[RoomDetailScreen] Failed to stop recording:", error);
      Alert.alert("停止失败", "无法停止当前录音。");
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.title}>{room.room.title}</Text>
      <Text style={styles.meta}>状态 {room.room.status}</Text>
      <Text style={styles.meta}>成员 {room.members.length}</Text>
      {room.active_recording ? (
        <Text style={styles.recording}>当前录音中 / Recording in progress</Text>
      ) : null}

      <View style={styles.buttonRow}>
        <PrimaryButton title="加入会议" onPress={() => void handleJoin()} style={styles.button} />
        <PrimaryButton title="离开会议" onPress={() => void handleLeave()} style={styles.button} />
      </View>
      <View style={styles.buttonRow}>
        <PrimaryButton title="开始录音" onPress={() => void handleStartRecording()} style={styles.button} />
        <PrimaryButton title="停止录音" onPress={() => void handleStopRecording()} style={styles.button} />
      </View>
      <PrimaryButton title="查看录音存档" onPress={() => navigation.navigate("Recordings")} style={styles.archiveButton} />

      {recording ? (
        <View style={styles.recordingCard}>
          <Text style={styles.sectionTitle}>最近录音产物</Text>
          {recording.files.map((file) => (
            <Text key={file.id} style={styles.fileLine}>
              {file.content_type} · {file.object_key}
            </Text>
          ))}
        </View>
      ) : null}

      <Text style={styles.sectionTitle}>成员</Text>
      <FlatList
        data={room.members}
        keyExtractor={(item) => String(item.id)}
        renderItem={({ item }) => (
          <View style={styles.card}>
            <Text style={styles.cardTitle}>用户 #{item.user_id}</Text>
            <Text style={styles.cardText}>{item.role}</Text>
            <Text style={styles.cardText}>
              {item.left_at ? `已离开 ${item.left_at}` : `加入于 ${item.joined_at ?? "未加入"}`}
            </Text>
          </View>
        )}
        onRefresh={() => void loadData()}
        refreshing={loading}
        ListFooterComponent={
          <View style={styles.eventsWrap}>
            <Text style={styles.sectionTitle}>最近事件</Text>
            {room.events.map((event) => (
              <View key={event.id} style={styles.card}>
                <Text style={styles.cardTitle}>{event.type}</Text>
                <Text style={styles.cardText}>{event.created_at}</Text>
              </View>
            ))}
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
  title: {
    fontSize: 24,
    fontWeight: "700",
    color: "#0f172a"
  },
  meta: {
    color: "#64748b",
    marginTop: 6
  },
  recording: {
    color: "#dc2626",
    marginTop: 8,
    fontWeight: "600"
  },
  buttonRow: {
    flexDirection: "row",
    gap: 12,
    marginTop: 16
  },
  button: {
    flex: 1
  },
  archiveButton: {
    marginTop: 12,
    backgroundColor: "#0f172a"
  },
  sectionTitle: {
    marginTop: 20,
    marginBottom: 10,
    fontSize: 16,
    fontWeight: "700",
    color: "#0f172a"
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 14,
    padding: 14,
    marginBottom: 10,
    borderWidth: 1,
    borderColor: "#e2e8f0"
  },
  cardTitle: {
    fontSize: 15,
    fontWeight: "600",
    color: "#0f172a"
  },
  cardText: {
    marginTop: 4,
    color: "#475569"
  },
  recordingCard: {
    backgroundColor: "#fff",
    borderRadius: 14,
    padding: 14,
    marginTop: 16,
    borderWidth: 1,
    borderColor: "#e2e8f0"
  },
  fileLine: {
    color: "#334155",
    marginTop: 6
  },
  eventsWrap: {
    paddingBottom: 24
  }
});

export default RoomDetailScreen;
