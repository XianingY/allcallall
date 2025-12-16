import React from "react";
import { View, Text, StyleSheet, TouchableOpacity, Dimensions } from "react-native";
import { RTCView } from "react-native-webrtc";

import { useSignaling } from "../context/SignalingContext";

const CallOverlay: React.FC = () => {
  const {
    status,
    session,
    acceptCall,
    rejectCall,
    endCall,
    localStream,
    remoteStream,
    isVideoEnabled,
    isAudioEnabled,
    toggleVideo,
    toggleAudio,
    switchCamera
  } = useSignaling();

  if (status === "idle" || !session) {
    return null;
  }

  const isIncoming = session.direction === "incoming";
  const hasRemoteVideo = (remoteStream?.getVideoTracks().length ?? 0) > 0;
  const hasLocalVideo = (localStream?.getVideoTracks().length ?? 0) > 0;
  const screenWidth = Dimensions.get("window").width;
  const screenHeight = Dimensions.get("window").height;

  return (
    <View style={styles.container}>
      {/* 远端视频（全屏） */}
      {hasRemoteVideo && remoteStream ? (
        <RTCView
          streamURL={remoteStream.toURL()}
          style={styles.remoteVideo}
          objectFit="cover"
          mirror={false}
        />
      ) : (
        <View style={styles.audioOnlyBackground}>
          <Text style={styles.audioOnlyText}>语音通话 / Audio Call</Text>
          <Text style={styles.peerEmail}>{session.peerEmail}</Text>
        </View>
      )}

      {/* 本地视频（小窗口） */}
      {hasLocalVideo && localStream && isVideoEnabled ? (
        <View style={styles.localVideoContainer}>
          <RTCView
            streamURL={localStream.toURL()}
            style={styles.localVideo}
            objectFit="cover"
            mirror={true}
          />
        </View>
      ) : null}

      {/* 状态信息 */}
      <View style={styles.statusBar}>
        <Text style={styles.statusText}>
          {status === "connecting"
            ? `正在呼叫 / Calling ${session.peerEmail}`
            : status === "incoming"
            ? `${session.peerEmail} 来电 / Incoming call`
            : `通话中 / In call`}
        </Text>
        {!isAudioEnabled && (
          <View style={styles.mutedIndicator}>
            <Text style={styles.mutedText}>🎙️ 麦克风已关闭 / Muted</Text>
          </View>
        )}
      </View>

      {/* 控制按钮 */}
      <View style={styles.controlsContainer}>
        {isIncoming && status === "incoming" ? (
          // 来电控制
          <>
            <TouchableOpacity
              style={[styles.controlButton, styles.acceptButton]}
              onPress={acceptCall}
            >
              <Text style={styles.controlButtonText}>✓</Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={[styles.controlButton, styles.rejectButton]}
              onPress={rejectCall}
            >
              <Text style={styles.controlButtonText}>✕</Text>
            </TouchableOpacity>
          </>
        ) : (
          // 通话中控制
          <>
            {/* 麦克风开关 */}
            <TouchableOpacity
              style={[
                styles.controlButton,
                !isAudioEnabled && styles.controlButtonDisabled
              ]}
              onPress={toggleAudio}
            >
              <Text style={styles.controlButtonText}>
                {isAudioEnabled ? "🎙️" : "🔇"}
              </Text>
            </TouchableOpacity>

            {/* 视频开关 */}
            <TouchableOpacity
              style={[
                styles.controlButton,
                !isVideoEnabled && styles.controlButtonDisabled
              ]}
              onPress={toggleVideo}
            >
              <Text style={styles.controlButtonText}>
                {isVideoEnabled ? "📹" : "🚫"}
              </Text>
            </TouchableOpacity>

            {/* 切换摄像头 */}
            {isVideoEnabled && (
              <TouchableOpacity
                style={styles.controlButton}
                onPress={switchCamera}
              >
                <Text style={styles.controlButtonText}>🔄</Text>
              </TouchableOpacity>
            )}

            {/* 结束通话 */}
            <TouchableOpacity
              style={[styles.controlButton, styles.endButton]}
              onPress={endCall}
            >
              <Text style={styles.controlButtonText}>✕</Text>
            </TouchableOpacity>
          </>
        )}
      </View>

      {/* 音频流隐藏处理 */}
      <View style={styles.audioAttachments}>
        {localStream && !isVideoEnabled ? (
          <RTCView streamURL={localStream.toURL()} style={styles.hiddenVideo} />
        ) : null}
        {remoteStream && !hasRemoteVideo ? (
          <RTCView streamURL={remoteStream.toURL()} style={styles.hiddenVideo} />
        ) : null}
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: "#000"
  },
  remoteVideo: {
    width: "100%",
    height: "100%"
  },
  audioOnlyBackground: {
    flex: 1,
    backgroundColor: "#1f2937",
    justifyContent: "center",
    alignItems: "center"
  },
  audioOnlyText: {
    color: "#fff",
    fontSize: 24,
    fontWeight: "700",
    marginBottom: 16
  },
  peerEmail: {
    color: "#9ca3af",
    fontSize: 16
  },
  localVideoContainer: {
    position: "absolute",
    top: 60,
    right: 20,
    width: 120,
    height: 160,
    borderRadius: 12,
    overflow: "hidden",
    borderWidth: 2,
    borderColor: "#fff",
    shadowColor: "#000",
    shadowOpacity: 0.3,
    shadowOffset: { width: 0, height: 2 },
    shadowRadius: 8,
    elevation: 5
  },
  localVideo: {
    width: "100%",
    height: "100%"
  },
  statusBar: {
    position: "absolute",
    top: 20,
    left: 20,
    right: 20,
    backgroundColor: "rgba(0,0,0,0.5)",
    borderRadius: 12,
    padding: 12
  },
  statusText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
    textAlign: "center"
  },
  mutedIndicator: {
    marginTop: 8,
    backgroundColor: "rgba(220,38,38,0.8)",
    borderRadius: 8,
    padding: 6
  },
  mutedText: {
    color: "#fff",
    fontSize: 14,
    textAlign: "center",
    fontWeight: "600"
  },
  controlsContainer: {
    position: "absolute",
    bottom: 40,
    left: 0,
    right: 0,
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    gap: 16,
    paddingHorizontal: 20
  },
  controlButton: {
    width: 64,
    height: 64,
    borderRadius: 32,
    backgroundColor: "rgba(255,255,255,0.3)",
    justifyContent: "center",
    alignItems: "center",
    shadowColor: "#000",
    shadowOpacity: 0.3,
    shadowOffset: { width: 0, height: 2 },
    shadowRadius: 4,
    elevation: 3
  },
  controlButtonDisabled: {
    backgroundColor: "rgba(220,38,38,0.8)"
  },
  controlButtonText: {
    fontSize: 28
  },
  acceptButton: {
    backgroundColor: "rgba(34,197,94,0.9)"
  },
  rejectButton: {
    backgroundColor: "rgba(220,38,38,0.9)"
  },
  endButton: {
    backgroundColor: "rgba(220,38,38,0.9)"
  },
  hiddenVideo: {
    width: 1,
    height: 1,
    opacity: 0
  },
  audioAttachments: {
    position: "absolute",
    width: 1,
    height: 1,
    top: 0,
    left: 0
  }
});

export default CallOverlay;
