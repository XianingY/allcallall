import React, { useEffect, useMemo, useState } from "react";
import { Alert, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import { RTCView } from "react-native-webrtc";

import { fetchRoomState, type MeetingJoinOptions, type RoomRecord } from "../api/collaboration";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { useRoomCall } from "../context/RoomCallContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "PreJoin">;

const PreJoinScreen: React.FC<Props> = ({ route, navigation }) => {
  const { token } = useAuthContext();
  const { localStream, preparePreview } = useRoomCall();
  const [room, setRoom] = useState<RoomRecord | null>(null);
  const [loading, setLoading] = useState(false);
  const [options, setOptions] = useState<MeetingJoinOptions>({
    audioEnabled: route.params.joinOptions?.audioEnabled ?? true,
    videoEnabled: route.params.joinOptions?.videoEnabled ?? true,
    cameraFacing: route.params.joinOptions?.cameraFacing ?? "front",
    speakerOn: route.params.joinOptions?.speakerOn ?? true,
  });

  useEffect(() => {
    if (!token) {
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        setLoading(true);
        const nextRoom = await fetchRoomState(token, route.params.roomId);
        if (!cancelled) {
          setRoom(nextRoom);
        }
      } catch (error) {
        console.error("[PreJoinScreen] Failed to load room:", error);
        if (!cancelled) {
          Alert.alert("加载失败", "无法读取会议详情。");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [route.params.roomId, token]);

  useEffect(() => {
    void preparePreview(options).catch((error) => {
      console.error("[PreJoinScreen] Failed to prepare preview:", error);
    });
  }, [options, preparePreview]);

  const previewUrl = useMemo(() => {
    try {
      return localStream?.toURL() ?? null;
    } catch {
      return null;
    }
  }, [localStream]);

  const handleJoin = async () => {
    if (!room) {
      Alert.alert("会议不可用", "当前会议房间还没有准备好。");
      return;
    }
    navigation.replace("RoomDetail", { room, joinOptions: options });
  };

  return (
    <View style={styles.container}>
      <Text style={styles.title}>{room?.room.title || route.params.title || "Join Meeting"}</Text>
      <Text style={styles.meta}>会议号 {route.params.roomId}</Text>
      {loading ? <Text style={styles.meta}>正在加载会议信息...</Text> : null}

      <View style={styles.previewCard}>
        {previewUrl ? (
          <RTCView streamURL={previewUrl} style={styles.preview} objectFit="cover" mirror />
        ) : (
          <View style={styles.previewPlaceholder}>
            <Text style={styles.previewText}>本地预览不可用</Text>
          </View>
        )}
      </View>

      <View style={styles.optionGrid}>
        <PrimaryButton
          title={options.audioEnabled ? "麦克风已开" : "麦克风已关"}
          onPress={() => setOptions((current) => ({ ...current, audioEnabled: !current.audioEnabled }))}
          style={options.audioEnabled ? styles.optionActive : styles.optionInactive}
        />
        <PrimaryButton
          title={options.videoEnabled ? "摄像头已开" : "摄像头已关"}
          onPress={() => setOptions((current) => ({ ...current, videoEnabled: !current.videoEnabled }))}
          style={options.videoEnabled ? styles.optionActive : styles.optionInactive}
        />
        <PrimaryButton
          title={options.cameraFacing === "front" ? "前置摄像头" : "后置摄像头"}
          onPress={() => setOptions((current) => ({ ...current, cameraFacing: current.cameraFacing === "front" ? "back" : "front" }))}
          style={styles.optionNeutral}
        />
        <PrimaryButton
          title={options.speakerOn ? "扬声器" : "听筒"}
          onPress={() => setOptions((current) => ({ ...current, speakerOn: !current.speakerOn }))}
          style={styles.optionNeutral}
        />
      </View>

      <View style={styles.infoCard}>
        <Text style={styles.infoText}>进入会议前可先完成设备检查。当前版本支持快速入会、音视频控制、成员列表和录音。</Text>
      </View>

      <PrimaryButton title="加入会议" onPress={() => void handleJoin()} style={styles.joinButton} />
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16,
  },
  title: {
    fontSize: 24,
    fontWeight: "700",
    color: "#0f172a",
  },
  meta: {
    color: "#64748b",
    marginTop: 6,
  },
  previewCard: {
    marginTop: 20,
    borderRadius: 20,
    overflow: "hidden",
    backgroundColor: "#0f172a",
    height: 280,
  },
  preview: {
    flex: 1,
  },
  previewPlaceholder: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  previewText: {
    color: "#cbd5e1",
    fontSize: 16,
  },
  optionGrid: {
    marginTop: 18,
    gap: 10,
  },
  optionActive: {
    backgroundColor: "#0f766e",
  },
  optionInactive: {
    backgroundColor: "#7f1d1d",
  },
  optionNeutral: {
    backgroundColor: "#334155",
  },
  infoCard: {
    marginTop: 16,
    borderRadius: 16,
    backgroundColor: "#ffffff",
    borderWidth: 1,
    borderColor: "#e2e8f0",
    padding: 16,
  },
  infoText: {
    color: "#334155",
    lineHeight: 20,
  },
  joinButton: {
    marginTop: 18,
    backgroundColor: "#0f172a",
  },
});

export default PreJoinScreen;
