import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import {
  createRoom,
  listRooms,
  type RoomRecord
} from "../api/collaboration";
import TextField from "../components/TextField";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "Rooms">;

const RoomsScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const [items, setItems] = useState<RoomRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [title, setTitle] = useState("");

  const loadData = useCallback(async () => {
    if (!token || !currentOrganization) {
      setItems([]);
      return;
    }
    try {
      setLoading(true);
      const data = await listRooms(token);
      setItems(data);
    } catch (error) {
      console.error("[RoomsScreen] Failed to load rooms:", error);
      Alert.alert("加载失败", "无法加载会议房间列表。");
    } finally {
      setLoading(false);
    }
  }, [currentOrganization, token]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const handleCreate = async () => {
    if (!token || !title.trim()) {
      return;
    }
    try {
      const room = await createRoom(token, { title: title.trim() });
      setTitle("");
      await loadData();
      navigation.navigate("RoomDetail", { room });
    } catch (error) {
      console.error("[RoomsScreen] Failed to create room:", error);
      Alert.alert("创建失败", "无法创建团队会议。");
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.heading}>{currentOrganization?.name ?? "当前工作区"} 会议房间</Text>
      <TextField
        label="新建会议房间"
        value={title}
        onChangeText={setTitle}
        placeholder="例如：APAC 周会"
      />
      <PrimaryButton title="创建会议" onPress={handleCreate} disabled={!title.trim()} style={styles.createButton} />
      <PrimaryButton title="查看录音存档" onPress={() => navigation.navigate("Recordings")} style={styles.secondaryButton} />

      <FlatList
        data={items}
        keyExtractor={(item) => String(item.room.id)}
        refreshing={loading}
        onRefresh={() => void loadData()}
        renderItem={({ item }) => (
          <TouchableOpacity style={styles.card} onPress={() => navigation.navigate("RoomDetail", { room: item })}>
            <View style={styles.row}>
              <Text style={styles.title}>{item.room.title}</Text>
              <Text style={styles.status}>{item.room.status}</Text>
            </View>
            <Text style={styles.meta}>成员 {item.members.length} / 事件 {item.events.length}</Text>
            {item.active_recording ? (
              <Text style={styles.recording}>录音中 / Recording</Text>
            ) : null}
          </TouchableOpacity>
        )}
        ListEmptyComponent={
          <View style={styles.empty}>
            <Text style={styles.emptyText}>当前工作区还没有会议房间。</Text>
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
  createButton: {
    marginBottom: 12
  },
  secondaryButton: {
    marginBottom: 16,
    backgroundColor: "#0f172a"
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "#e2e8f0"
  },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center"
  },
  title: {
    fontSize: 17,
    fontWeight: "600",
    color: "#0f172a"
  },
  status: {
    color: "#2563eb",
    fontWeight: "600"
  },
  meta: {
    color: "#64748b",
    marginTop: 8
  },
  recording: {
    color: "#dc2626",
    marginTop: 8,
    fontWeight: "600"
  },
  empty: {
    alignItems: "center",
    paddingTop: 48
  },
  emptyText: {
    color: "#64748b"
  }
});

export default RoomsScreen;
