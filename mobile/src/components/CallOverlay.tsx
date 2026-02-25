import React, { useCallback, useEffect } from "react";
import { View, Text, StyleSheet, TouchableOpacity, Dimensions, Alert } from "react-native";
import { RTCView } from "react-native-webrtc";
import { activateKeepAwake, deactivateKeepAwake } from "expo-keep-awake";

import { useSignaling } from "../context/SignalingContext";
import { useSubtitleStore } from "../store/useSubtitleStore";
import TranslationOverlay from "./translation/TranslationOverlay";
import TranslationControl from "./translation/TranslationControl";

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
    isRemoteVideoEnabled,
    isRemoteAudioEnabled,
    toggleVideo,
    toggleAudio,
    switchCamera,
    toggleSpeaker,
    isSpeakerOn,
    networkQuality,
    translationEnabled,
    translationLanguage,
    toggleTranslation,
    setTranslationLanguage
  } = useSignaling();

  const subtitles = useSubtitleStore((state) => state.subtitles);
  const clearSubtitles = useSubtitleStore((state) => state.clearSubtitles);

  useEffect(() => {
    const tag = "call-overlay";
    if (status !== "idle" && session) {
      activateKeepAwake(tag);
    } else {
      deactivateKeepAwake(tag);
    }

    return () => {
      deactivateKeepAwake(tag);
    };
  }, [session, status]);

  if (status === "idle" || !session) {
    return null;
  }

  const isIncoming = session.direction === "incoming";
  const hasRemoteVideo = (remoteStream?.getVideoTracks().length ?? 0) > 0;
  const hasLocalVideo = (localStream?.getVideoTracks().length ?? 0) > 0;
  const screenWidth = Dimensions.get("window").width;
  const screenHeight = Dimensions.get("window").height;

  // 视频开关处理 - 添加带宽警告
  const handleVideoToggle = useCallback(() => {
    if (!isVideoEnabled) {
      // 开启视频前显示警告
      Alert.alert(
        "视频通话提示 / Video Call Notice",
        "视频通话需要较高带宽，可能导致通话不稳定或中断。建议使用 WiFi 网络。\n\nVideo calls require high bandwidth and may cause unstable connections. WiFi is recommended.",
        [
          { text: "取消 / Cancel", style: "cancel" },
          { text: "开启视频 / Enable", onPress: () => toggleVideo() }
        ]
      );
    } else {
      toggleVideo();
    }
  }, [isVideoEnabled, toggleVideo]);

  const getNetworkQualityColor = (quality: string) => {
    switch (quality) {
      case "excellent": return "#22c55e"; // Green
      case "good": return "#84cc16";      // Lime
      case "poor": return "#f59e0b";      // Amber
      case "bad": return "#ef4444";       // Red
      default: return "#9ca3af";          // Gray
    }
  };

  return (
    <View style={styles.container}>
      {/* 远端视频（全屏） */}
      {hasRemoteVideo && remoteStream && isRemoteVideoEnabled ? (
        <RTCView
          streamURL={remoteStream.toURL()}
          style={styles.remoteVideo}
          objectFit="cover"
          mirror={false}
        />
      ) : (
        <View style={styles.audioOnlyBackground}>
          <Text style={styles.audioOnlyText}>
            {!isRemoteVideoEnabled && hasRemoteVideo
              ? "对方已关闭摄像头 / Camera Off"
              : "语音通话 / Audio Call"}
          </Text>
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
      
      {/* 网络质量指示器 */}
      <View style={styles.networkIndicator}>
        <View style={[styles.networkDot, { backgroundColor: getNetworkQualityColor(networkQuality) }]} />
        <Text style={styles.networkText}>
          {networkQuality === "excellent" ? "网络极佳 / Excellent" :
           networkQuality === "good" ? "网络良好 / Good" :
           networkQuality === "poor" ? "网络较差 / Poor" :
           networkQuality === "bad" ? "网络极差 / Bad" :
           "检测中... / Detecting..."}
        </Text>
      </View>

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
        {!isRemoteAudioEnabled && (
          <View style={[styles.mutedIndicator, { backgroundColor: "rgba(245,158,11,0.8)" }]}>
            <Text style={styles.mutedText}>🔇 对方已静音 / Remote Muted</Text>
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

            {/* 扬声器开关 */}
            <TouchableOpacity
              style={[
                styles.controlButton,
                !isSpeakerOn && { backgroundColor: "rgba(255,255,255,0.3)" }
              ]}
              onPress={toggleSpeaker}
            >
              <Text style={styles.controlButtonText}>
                {isSpeakerOn ? "🔊" : "👂"}
              </Text>
            </TouchableOpacity>

            {/* 视频开关 */}
            <TouchableOpacity
              style={[
                styles.controlButton,
                !isVideoEnabled && styles.controlButtonDisabled
              ]}
              onPress={handleVideoToggle}
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

      {/* 翻译字幕显示 */}
      {status === "in_call" && (
        <>
          <TranslationOverlay
            subtitles={subtitles}
            isVisible={translationEnabled}
            language={translationLanguage}
            onClear={clearSubtitles}
          />

          {/* 翻译提示信息 (未开启时显示) */}
          {!translationEnabled && (
            <View style={styles.translationHint}>
              <Text style={styles.translationHintText}>
                💡 点击下方"实时翻译"开关开启翻译字幕
              </Text>
            </View>
          )}

          {/* 翻译控制面板 */}
          <View style={styles.translationControlContainer}>
            <TranslationControl
              isEnabled={translationEnabled}
              onToggle={toggleTranslation}
              targetLanguage={translationLanguage}
              onLanguageChange={setTranslationLanguage}
            />
          </View>
        </>
      )}

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
  networkIndicator: {
    position: "absolute",
    top: 60,
    left: 20,
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: "rgba(0,0,0,0.5)",
    padding: 8,
    borderRadius: 8
  },
  networkDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    marginRight: 6
  },
  networkText: {
    color: "#fff",
    fontSize: 12,
    fontWeight: "600"
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
  },
  translationControlContainer: {
    position: "absolute",
    bottom: 100,
    left: 0,
    right: 0,
    zIndex: 999
  },
  translationHint: {
    position: "absolute",
    bottom: 180,
    left: 20,
    right: 20,
    backgroundColor: "rgba(59, 130, 246, 0.9)",
    borderRadius: 12,
    padding: 12,
    zIndex: 998
  },
  translationHintText: {
    color: "#fff",
    fontSize: 14,
    textAlign: "center",
    fontWeight: "600"
  }
});

export default CallOverlay;
