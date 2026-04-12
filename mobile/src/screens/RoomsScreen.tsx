import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, FlatList, Pressable, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { createRoom, listRecordings, listRooms, type ConversationRecord, type RecordingRecord, type RoomRecord } from "../api/collaboration";
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
  const [recordings, setRecordings] = useState<RecordingRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [title, setTitle] = useState("");
  const [joinCode, setJoinCode] = useState("");

  const loadData = useCallback(async () => {
    if (!token || !currentOrganization) {
      setItems([]);
      return;
    }
    try {
      setLoading(true);
      const [rooms, recordingItems] = await Promise.all([
        listRooms(token),
        listRecordings(token),
      ]);
      setItems(rooms);
      setRecordings(recordingItems);
    } catch (error) {
      console.error("[RoomsScreen] Failed to load rooms:", error);
      Alert.alert("加载失败", "无法加载会议列表。");
    } finally {
      setLoading(false);
    }
  }, [currentOrganization, token]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const activeRooms = useMemo(() => items.filter((item) => item.is_active), [items]);
  const upcomingRooms = useMemo(() => items.filter((item) => !item.is_active && item.room.status === "scheduled"), [items]);
  const recentRooms = useMemo(() => items.slice(0, 6), [items]);
  const recentRecordings = useMemo(() => recordings.slice(0, 3), [recordings]);
  const roomMap = useMemo(() => new Map(items.map((item) => [item.room.id, item])), [items]);

  const buildConversationTarget = useCallback((room: RoomRecord): ConversationRecord | null => {
    if (!room.conversation_id) {
      return null;
    }
    return {
      id: room.conversation_id,
      organization_id: room.room.organization_id,
      team_id: room.room.team_id ?? null,
      room_id: room.room.id,
      type: "meeting",
      title: room.conversation_title || room.room.title,
      topic: "",
      status: "open",
      priority: "normal",
      unread_count: 0,
      active_room_id: room.is_active ? room.room.id : null,
      active_room_title: room.is_active ? room.room.title : "",
      latest_room_id: room.room.id,
      latest_room_title: room.room.title,
      latest_recording_id: room.latest_recording_id ?? null,
    };
  }, []);

  const handleCreate = async () => {
    if (!token || !title.trim()) {
      return;
    }
    try {
      const room = await createRoom(token, { title: title.trim() });
      setTitle("");
      await loadData();
      navigation.navigate("PreJoin", {
        roomId: room.room.id,
        title: room.room.title,
        conversationId: room.conversation_id ?? null,
        joinOptions: {
          audioEnabled: true,
          videoEnabled: true,
          cameraFacing: "front",
          speakerOn: true,
        },
      });
    } catch (error) {
      console.error("[RoomsScreen] Failed to create room:", error);
      Alert.alert("创建失败", "无法发起即时会议。");
    }
  };

  const handleJoinByCode = () => {
    const roomId = Number(joinCode.trim());
    if (!Number.isFinite(roomId) || roomId <= 0) {
      Alert.alert("会议号无效", "请输入正确的会议房间 ID。");
      return;
    }
    navigation.navigate("PreJoin", {
      roomId,
      joinOptions: {
        audioEnabled: true,
        videoEnabled: true,
        cameraFacing: "front",
        speakerOn: true,
      },
    });
  };

  const renderRoomCard = (room: RoomRecord) => (
    <Pressable
      key={room.room.id}
      style={styles.card}
      onPress={() => navigation.navigate("PreJoin", {
        roomId: room.room.id,
        title: room.room.title,
        conversationId: room.conversation_id ?? null,
        joinOptions: {
          audioEnabled: true,
          videoEnabled: true,
          cameraFacing: "front",
          speakerOn: true,
        },
      })}
    >
      <View style={styles.cardRow}>
        <Text style={styles.cardTitle}>{room.room.title}</Text>
        <Text style={styles.badge}>{room.is_active ? "LIVE" : room.room.status.toUpperCase()}</Text>
      </View>
      <Text style={styles.cardMeta}>参会人数 {room.participant_count} · 事件 {room.events.length}</Text>
      {room.conversation_title ? <Text style={styles.cardMeta}>所属线程 {room.conversation_title}</Text> : null}
      {room.has_recording ? <Text style={styles.recordingMeta}>已有录音资产</Text> : null}
      {!room.is_active && room.conversation_id ? (
        <PrimaryButton
          title="回到线程摘要"
          onPress={() => {
            const target = buildConversationTarget(room);
            if (!target) {
              return;
            }
            navigation.navigate("ConversationDetail", { conversation: target });
          }}
          style={styles.threadButton}
        />
      ) : null}
    </Pressable>
  );

  return (
    <View style={styles.container}>
      <Text style={styles.heading}>{currentOrganization?.name ?? "当前工作区"} Meetings</Text>
      <Text style={styles.subheading}>发起、加入、继续进行中的会议。</Text>

      <View style={styles.quickActionsCard}>
        <Text style={styles.sectionTitle}>Quick actions</Text>
        <TextField
          label="发起即时会议"
          value={title}
          onChangeText={setTitle}
          placeholder="例如：跨境客服晨会"
        />
        <PrimaryButton title="Start instant meeting" onPress={() => void handleCreate()} disabled={!title.trim()} />
        <TextField
          label="按会议号加入"
          value={joinCode}
          onChangeText={setJoinCode}
          placeholder="输入 room ID"
        />
        <PrimaryButton title="Join by room ID" onPress={handleJoinByCode} disabled={!joinCode.trim()} style={styles.secondaryAction} />
      </View>

      <View style={styles.navRow}>
        <PrimaryButton title="Inbox" onPress={() => navigation.navigate("Conversations")} style={styles.navButton} />
        <PrimaryButton title="Contacts" onPress={() => navigation.navigate("Contacts")} style={styles.navButton} />
        <PrimaryButton title="FollowUps" onPress={() => navigation.navigate("FollowUps")} style={styles.navButton} />
        <PrimaryButton title="Settings" onPress={() => navigation.navigate("Settings")} style={styles.navButton} />
      </View>

      <FlatList
        data={recentRooms}
        keyExtractor={(item) => String(item.room.id)}
        refreshing={loading}
        onRefresh={() => void loadData()}
        ListHeaderComponent={
          <View>
            <Text style={styles.sectionTitle}>Active / Upcoming</Text>
            {activeRooms.length > 0 ? activeRooms.map(renderRoomCard) : null}
            {upcomingRooms.length > 0 ? upcomingRooms.slice(0, 3).map(renderRoomCard) : null}
            {recentRecordings.length > 0 ? (
              <View>
                <Text style={styles.sectionTitle}>Recent recording assets</Text>
                {recentRecordings.map((recording) => {
                  const room = roomMap.get(recording.session.room_id);
                  return (
                    <View key={recording.session.id} style={styles.assetCard}>
                      <Text style={styles.cardTitle}>{room?.room.title || `会议 #${recording.session.room_id}`}</Text>
                      <Text style={styles.cardMeta}>
                        录音会话 #{recording.session.id} · 文件 {recording.files.length} 个
                      </Text>
                      <Text style={styles.cardMeta}>
                        最近产物 {recording.files[0]?.recording_kind || "mixed_audio"} · {recording.files[0]?.duration_seconds || 0}s
                      </Text>
                      <View style={styles.assetActions}>
                        <PrimaryButton
                          title="查看录音资产"
                          onPress={() => navigation.navigate("Recordings")}
                          style={styles.assetButton}
                        />
                        {room?.conversation_id ? (
                          <PrimaryButton
                            title="回到线程摘要"
                            onPress={() => {
                              const target = buildConversationTarget(room);
                              if (!target) {
                                return;
                              }
                              navigation.navigate("ConversationDetail", { conversation: target });
                            }}
                            style={styles.assetButtonSecondary}
                          />
                        ) : null}
                      </View>
                    </View>
                  );
                })}
              </View>
            ) : null}
            <Text style={styles.sectionTitle}>Recent</Text>
          </View>
        }
        renderItem={({ item }) => renderRoomCard(item)}
        ListEmptyComponent={<Text style={styles.empty}>当前工作区还没有会议。</Text>}
      />
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16,
  },
  heading: {
    fontSize: 24,
    fontWeight: "700",
    color: "#0f172a",
  },
  subheading: {
    marginTop: 6,
    color: "#475569",
    marginBottom: 14,
  },
  quickActionsCard: {
    backgroundColor: "#ffffff",
    borderRadius: 18,
    padding: 16,
    borderWidth: 1,
    borderColor: "#e2e8f0",
    marginBottom: 14,
  },
  sectionTitle: {
    fontSize: 16,
    fontWeight: "700",
    color: "#0f172a",
    marginBottom: 10,
  },
  secondaryAction: {
    marginTop: 10,
    backgroundColor: "#334155",
  },
  navRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 10,
    marginBottom: 14,
  },
  navButton: {
    backgroundColor: "#1e293b",
    minWidth: 110,
  },
  card: {
    backgroundColor: "#ffffff",
    borderRadius: 16,
    padding: 16,
    borderWidth: 1,
    borderColor: "#e2e8f0",
    marginBottom: 12,
  },
  cardRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  cardTitle: {
    color: "#0f172a",
    fontWeight: "700",
    flex: 1,
    marginRight: 12,
  },
  badge: {
    color: "#1d4ed8",
    fontWeight: "700",
  },
  cardMeta: {
    color: "#64748b",
    marginTop: 6,
  },
  recordingMeta: {
    color: "#991b1b",
    marginTop: 6,
    fontWeight: "600",
  },
  threadButton: {
    marginTop: 12,
    backgroundColor: "#334155",
  },
  assetCard: {
    backgroundColor: "#ffffff",
    borderRadius: 16,
    padding: 16,
    borderWidth: 1,
    borderColor: "#e2e8f0",
    marginBottom: 12,
  },
  assetActions: {
    flexDirection: "row",
    gap: 10,
    marginTop: 12,
  },
  assetButton: {
    flex: 1,
    backgroundColor: "#0f172a",
  },
  assetButtonSecondary: {
    flex: 1,
    backgroundColor: "#475569",
  },
  empty: {
    color: "#64748b",
    textAlign: "center",
    marginTop: 40,
  },
});

export default RoomsScreen;
