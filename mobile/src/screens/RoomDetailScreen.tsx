import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  FlatList,
  ScrollView,
  StyleSheet,
  Text,
  View,
  useWindowDimensions,
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import { RTCView } from "react-native-webrtc";
import * as Clipboard from "expo-clipboard";

import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { useRoomCall } from "../context/RoomCallContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import ChatRealtimeService from "../services/ChatRealtimeService";

type Props = NativeStackScreenProps<RootStackParamList, "RoomDetail">;

type GalleryTile = {
  id: string;
  label: string;
  email?: string;
  streamURL: string | null;
  isLocal?: boolean;
  audioEnabled?: boolean;
  videoEnabled?: boolean;
  isHost?: boolean;
};

const RoomDetailScreen: React.FC<Props> = ({ route, navigation }) => {
  const {
    room,
    localStream,
    remoteStreams,
    deviceState,
    controlState,
    recording,
    joinMeeting,
    leaveMeeting,
    toggleAudio,
    toggleVideo,
    switchCamera,
    toggleSpeaker,
    refreshRoom,
    startRecording,
    stopRecording,
  } = useRoomCall();
  const { token, user } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const { width } = useWindowDimensions();
  const [pageIndex, setPageIndex] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        await joinMeeting(route.params.room.room.id, route.params.joinOptions ?? {
          audioEnabled: true,
          videoEnabled: true,
          cameraFacing: "front",
          speakerOn: true,
        });
      } catch (error) {
        console.error("[RoomDetailScreen] Failed to join meeting:", error);
        if (!cancelled) {
          Alert.alert("加入失败", "当前无法加入会议。");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [joinMeeting, route.params.joinOptions, route.params.room.room.id]);

  useEffect(() => {
    const timer = setInterval(() => {
      void refreshRoom(route.params.room.room.id);
    }, 5000);
    return () => clearInterval(timer);
  }, [refreshRoom, route.params.room.room.id]);

  useEffect(() => {
    if (!token || !currentOrganization) {
      return;
    }
    const handleOpen = () => {
      void refreshRoom(route.params.room.room.id);
    };
    const handleEvent = (event: { event: string; payload: unknown }) => {
      const payload = event.payload as { room_id?: number } | undefined;
      if (!payload?.room_id || payload.room_id !== route.params.room.room.id) {
        return;
      }
      if (event.event === "room.updated" || event.event === "room.media.updated") {
        void refreshRoom(route.params.room.room.id);
      }
    };
    ChatRealtimeService.connect(token, currentOrganization.id);
    ChatRealtimeService.on("open", handleOpen);
    ChatRealtimeService.on("event", handleEvent);
    return () => {
      ChatRealtimeService.off("open", handleOpen);
      ChatRealtimeService.off("event", handleEvent);
    };
  }, [currentOrganization, refreshRoom, route.params.room.room.id, token]);

  const currentRoom = room ?? route.params.room;
  const roomWidth = Math.max(width - 32, 280);
  const localPreviewUrl = useMemo(() => {
    try {
      return localStream?.toURL() ?? null;
    } catch {
      return null;
    }
  }, [localStream]);

  const galleryTiles = useMemo<GalleryTile[]>(() => {
    const tiles: GalleryTile[] = [];
    const currentUserMember = currentRoom.members.find((member) => member.user_id === user?.id);
    tiles.push({
      id: "local",
      label: "You",
      email: user?.email,
      streamURL: localPreviewUrl,
      isLocal: true,
      audioEnabled: deviceState.audioEnabled,
      videoEnabled: deviceState.videoEnabled,
      isHost: currentUserMember?.is_host ?? false,
    });
    remoteStreams.forEach((item, index) => {
      const member = currentRoom.members.find((candidate) => candidate.user_id !== user?.id && candidate.connection_state !== "left" && candidate.joined_at) ?? currentRoom.members[index + 1];
      tiles.push({
        id: item.id,
        label: member?.user_display_name || member?.user_email || `Participant ${index + 1}`,
        email: member?.user_email,
        streamURL: item.stream.toURL(),
        audioEnabled: member?.audio_enabled,
        videoEnabled: member?.video_enabled,
        isHost: member?.is_host,
      });
    });
    return tiles;
  }, [currentRoom.members, deviceState.audioEnabled, deviceState.videoEnabled, localPreviewUrl, remoteStreams, user?.email, user?.id]);

  const galleryPages = useMemo(() => {
    if (galleryTiles.length <= 4) {
      return [galleryTiles];
    }
    const pages: GalleryTile[][] = [];
    for (let index = 0; index < galleryTiles.length; index += 4) {
      pages.push(galleryTiles.slice(index, index + 4));
    }
    return pages;
  }, [galleryTiles]);

  const handleLeave = async () => {
    await leaveMeeting();
    if (currentRoom.conversation_id) {
      navigation.replace("ConversationDetail", {
        conversation: {
          id: currentRoom.conversation_id,
          organization_id: currentRoom.room.organization_id,
          team_id: currentRoom.room.team_id ?? null,
          room_id: currentRoom.room.id,
          type: "meeting",
          title: currentRoom.conversation_title || currentRoom.room.title,
          topic: "",
          status: "open",
          priority: "normal",
          unread_count: 0,
        },
      });
      return;
    }
    navigation.replace("Rooms");
  };

  const handleCopyMeetingLink = async () => {
    await Clipboard.setStringAsync(`allcallall://rooms/${currentRoom.room.id}`);
    Alert.alert("已复制", "会议链接已复制到剪贴板。");
  };

  const currentUserMembership = currentRoom.members.find((member) => member.user_id === user?.id);
  const canRecord = currentUserMembership?.is_host ?? false;

  const renderGalleryTile = (tile: GalleryTile, variant: "single" | "grid") => (
    <View
      key={tile.id}
      style={[
        styles.tile,
        variant === "single" ? styles.singleTile : styles.gridTile,
      ]}
    >
      {tile.streamURL && tile.videoEnabled !== false ? (
        <RTCView streamURL={tile.streamURL} style={styles.video} objectFit="cover" mirror={tile.isLocal} />
      ) : (
        <View style={styles.placeholderTile}>
          <Text style={styles.placeholderInitial}>
            {(tile.label || "?").slice(0, 1).toUpperCase()}
          </Text>
        </View>
      )}
      <View style={styles.tileOverlay}>
        <Text style={styles.tileLabel}>{tile.label}</Text>
        <Text style={styles.tileMeta}>
          {tile.isHost ? "Host" : "Participant"} · {tile.audioEnabled === false ? "Muted" : "Mic on"} · {tile.videoEnabled === false ? "Video off" : "Video on"}
        </Text>
      </View>
    </View>
  );

  const renderGallery = () => {
    if (galleryTiles.length <= 1) {
      return (
        <View style={styles.singleGallery}>
          {renderGalleryTile(galleryTiles[0], "single")}
        </View>
      );
    }

    if (galleryTiles.length <= 4) {
      return (
        <View style={styles.gridGallery}>
          {galleryTiles.map((tile) => renderGalleryTile(tile, "grid"))}
        </View>
      );
    }

    return (
      <View>
        <ScrollView
          horizontal
          pagingEnabled
          showsHorizontalScrollIndicator={false}
          onMomentumScrollEnd={(event) => {
            const nextPage = Math.round(event.nativeEvent.contentOffset.x / roomWidth);
            setPageIndex(nextPage);
          }}
        >
          {galleryPages.map((page, index) => (
            <View key={`page-${index}`} style={[styles.galleryPage, { width: roomWidth }]}>
              <View style={styles.gridGallery}>
                {page.map((tile) => renderGalleryTile(tile, "grid"))}
              </View>
            </View>
          ))}
        </ScrollView>
        <Text style={styles.pageIndicator}>
          {pageIndex + 1} / {galleryPages.length}
        </Text>
      </View>
    );
  };

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.title}>{currentRoom.room.title}</Text>
        <Text style={styles.meta}>
          {controlState.connectionState === "connected" ? "Connected" : controlState.connectionState} · {currentRoom.participant_count} 人
        </Text>
        <Text style={styles.meta}>
          {currentRoom.active_recording ? "Recording live" : "Recording idle"} · {canRecord ? "Host controls enabled" : "Participant"}
        </Text>
      </View>

      {renderGallery()}

      <View style={styles.summaryCard}>
        <Text style={styles.summaryText}>会议状态 {currentRoom.room.status}</Text>
        {currentRoom.conversation_title ? <Text style={styles.summaryText}>所属线程 {currentRoom.conversation_title}</Text> : null}
        {recording?.files?.length ? <Text style={styles.summaryText}>本次录音产物 {recording.files.length} 个</Text> : null}
      </View>

      <View style={styles.controlRow}>
        <PrimaryButton
          title={deviceState.audioEnabled ? "静音" : "开麦"}
          onPress={toggleAudio}
          style={styles.controlButton}
        />
        <PrimaryButton
          title={deviceState.videoEnabled ? "关视频" : "开视频"}
          onPress={() => void toggleVideo()}
          style={styles.controlButton}
        />
        <PrimaryButton
          title="切换摄像头"
          onPress={() => void switchCamera()}
          style={styles.controlButton}
        />
      </View>
      <View style={styles.controlRow}>
        <PrimaryButton
          title={deviceState.speakerOn ? "扬声器" : "听筒"}
          onPress={() => void toggleSpeaker()}
          style={styles.controlButton}
        />
        <PrimaryButton
          title="成员"
          onPress={() => navigation.navigate("MeetingParticipants", {
            roomId: currentRoom.room.id,
            title: currentRoom.room.title,
          })}
          style={styles.controlButton}
        />
        <PrimaryButton
          title={currentRoom.active_recording ? "停止录音" : "开始录音"}
          onPress={() => void (currentRoom.active_recording ? stopRecording() : startRecording())}
          style={canRecord ? styles.controlButton : styles.disabledButton}
          disabled={!canRecord}
        />
      </View>
      <View style={styles.controlRow}>
        <PrimaryButton
          title="复制会议链接"
          onPress={() => void handleCopyMeetingLink()}
          style={styles.secondaryButton}
        />
        {currentRoom.conversation_id ? (
          <PrimaryButton
            title="回到线程摘要"
            onPress={() => navigation.navigate("ConversationDetail", {
              conversation: {
                id: currentRoom.conversation_id!,
                organization_id: currentRoom.room.organization_id,
                team_id: currentRoom.room.team_id ?? null,
                room_id: currentRoom.room.id,
                type: "meeting",
                title: currentRoom.conversation_title || currentRoom.room.title,
                topic: "",
                status: "open",
                priority: "normal",
                unread_count: 0,
              },
            })}
            style={styles.secondaryButton}
          />
        ) : null}
        <PrimaryButton
          title="离开会议"
          onPress={() => void handleLeave()}
          style={styles.leaveButton}
        />
      </View>

      <FlatList
        data={currentRoom.events}
        keyExtractor={(item) => String(item.id)}
        contentContainerStyle={styles.eventsList}
        renderItem={({ item }) => (
          <View style={styles.eventCard}>
            <Text style={styles.eventType}>{item.type}</Text>
            <Text style={styles.eventTime}>{new Date(item.created_at).toLocaleString()}</Text>
          </View>
        )}
        ListHeaderComponent={<Text style={styles.eventsHeading}>最近事件</Text>}
      />
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#020617",
    padding: 16,
  },
  header: {
    marginBottom: 14,
  },
  title: {
    color: "#f8fafc",
    fontSize: 24,
    fontWeight: "700",
  },
  meta: {
    color: "#cbd5e1",
    marginTop: 6,
  },
  singleGallery: {
    height: 360,
  },
  gridGallery: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 10,
  },
  galleryPage: {
    paddingRight: 8,
  },
  tile: {
    backgroundColor: "#1e293b",
    borderRadius: 18,
    overflow: "hidden",
    justifyContent: "center",
  },
  singleTile: {
    flex: 1,
  },
  gridTile: {
    width: "48%",
    aspectRatio: 1,
  },
  video: {
    flex: 1,
  },
  placeholderTile: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "#0f172a",
  },
  placeholderInitial: {
    color: "#e2e8f0",
    fontSize: 42,
    fontWeight: "700",
  },
  tileOverlay: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 0,
    padding: 10,
    backgroundColor: "rgba(2, 6, 23, 0.72)",
  },
  tileLabel: {
    color: "#f8fafc",
    fontWeight: "700",
  },
  tileMeta: {
    color: "#cbd5e1",
    fontSize: 12,
    marginTop: 4,
  },
  pageIndicator: {
    color: "#94a3b8",
    textAlign: "center",
    marginTop: 8,
    marginBottom: 4,
  },
  summaryCard: {
    backgroundColor: "#0f172a",
    borderRadius: 16,
    padding: 14,
    marginTop: 16,
    borderWidth: 1,
    borderColor: "#1e293b",
  },
  summaryText: {
    color: "#cbd5e1",
    marginTop: 4,
  },
  controlRow: {
    flexDirection: "row",
    gap: 10,
    marginTop: 12,
  },
  controlButton: {
    flex: 1,
    backgroundColor: "#1e293b",
  },
  secondaryButton: {
    flex: 1,
    backgroundColor: "#334155",
  },
  disabledButton: {
    flex: 1,
    backgroundColor: "#475569",
  },
  leaveButton: {
    flex: 1,
    backgroundColor: "#991b1b",
  },
  eventsList: {
    paddingBottom: 24,
    marginTop: 10,
  },
  eventsHeading: {
    color: "#f8fafc",
    fontWeight: "700",
    marginBottom: 10,
  },
  eventCard: {
    backgroundColor: "#0f172a",
    borderRadius: 14,
    padding: 12,
    marginBottom: 10,
  },
  eventType: {
    color: "#f8fafc",
    fontWeight: "600",
  },
  eventTime: {
    color: "#94a3b8",
    marginTop: 4,
  },
});

export default RoomDetailScreen;
