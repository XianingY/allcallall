import React, { useMemo } from "react";
import { FlatList, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { useAuthContext } from "../context/AuthContext";
import { useRoomCall } from "../context/roomCallContextValue";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "MeetingParticipants">;

const MeetingParticipantsScreen: React.FC<Props> = () => {
  const { user } = useAuthContext();
  const { room, deviceState } = useRoomCall();

  const items = useMemo(() => {
    return (room?.members ?? []).map((member) => {
      const isSelf = member.user_id === user?.id;
      return {
        ...member,
        audio_enabled: isSelf ? deviceState.audioEnabled : member.audio_enabled,
        video_enabled: isSelf ? deviceState.videoEnabled : member.video_enabled,
      };
    });
  }, [deviceState.audioEnabled, deviceState.videoEnabled, room?.members, user?.id]);

  return (
    <View style={styles.container}>
      <Text style={styles.heading}>参会成员</Text>
      <FlatList
        data={items}
        keyExtractor={(item) => String(item.id)}
        renderItem={({ item }) => (
          <View style={styles.card}>
            <View style={styles.row}>
              <Text style={styles.name}>{item.user_display_name || item.user_email || `成员 #${item.user_id}`}</Text>
              {item.is_host ? <Text style={styles.host}>HOST</Text> : null}
            </View>
            <Text style={styles.meta}>{item.user_email}</Text>
            <Text style={styles.meta}>状态 {item.connection_state || "unknown"}</Text>
            <Text style={styles.meta}>麦克风 {item.audio_enabled ? "开启" : "关闭"} · 摄像头 {item.video_enabled ? "开启" : "关闭"}</Text>
          </View>
        )}
        ListEmptyComponent={<Text style={styles.empty}>当前会议还没有可见成员。</Text>}
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
    fontSize: 22,
    fontWeight: "700",
    color: "#0f172a",
    marginBottom: 12,
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "#e2e8f0",
  },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  name: {
    color: "#0f172a",
    fontWeight: "700",
    flex: 1,
    marginRight: 12,
  },
  host: {
    color: "#1d4ed8",
    fontWeight: "700",
  },
  meta: {
    color: "#475569",
    marginTop: 6,
  },
  empty: {
    color: "#64748b",
    textAlign: "center",
    marginTop: 48,
  },
});

export default MeetingParticipantsScreen;
