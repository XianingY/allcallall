import React from "react";
import { View, Text, StyleSheet, TouchableOpacity } from "react-native";
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
    remoteStream
  } = useSignaling();

  if (status === "idle" || !session) {
    return null;
  }

  const isIncoming = session.direction === "incoming";

  return (
    <View style={styles.container} pointerEvents="box-none">
      <View style={styles.audioAttachments} pointerEvents="none">
        {localStream ? (
          <RTCView streamURL={localStream.toURL()} style={styles.hiddenVideo} />
        ) : null}
        {remoteStream ? (
          <RTCView streamURL={remoteStream.toURL()} style={styles.hiddenVideo} />
        ) : null}
      </View>
      <View style={styles.card}>
        <Text style={styles.title}>
          {status === "connecting"
            ? `正在呼叫 ${session.peerEmail} / Calling ${session.peerEmail}`
            : status === "incoming"
            ? `${session.peerEmail} 呼叫你 / ${session.peerEmail} is calling`
            : `与 ${session.peerEmail} 通话中 / In call with ${session.peerEmail}`}
        </Text>
        <Text style={styles.subtitle}>
          状态 / Status: {status === "connecting" ? "呼叫中 / Connecting" : status === "incoming" ? "来电 / Incoming" : "通话中 / In call"}
        </Text>
        <View style={styles.actions}>
          {isIncoming && status === "incoming" ? (
            <>
              <TouchableOpacity
                style={[styles.button, styles.accept]}
                onPress={acceptCall}
              >
                <Text style={styles.buttonText}>接受 / Accept</Text>
              </TouchableOpacity>
              <TouchableOpacity
                style={[styles.button, styles.reject]}
                onPress={rejectCall}
              >
                <Text style={styles.buttonText}>拒绝 / Reject</Text>
              </TouchableOpacity>
            </>
          ) : (
            <TouchableOpacity
              style={[styles.button, styles.reject]}
              onPress={endCall}
            >
              <Text style={styles.buttonText}>结束 / End</Text>
            </TouchableOpacity>
          )}
        </View>
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 20,
    paddingHorizontal: 20
  },
  card: {
    backgroundColor: "#111827",
    borderRadius: 20,
    padding: 20,
    shadowColor: "#000",
    shadowOpacity: 0.2,
    shadowOffset: { width: 0, height: 6 },
    shadowRadius: 12,
    elevation: 6
  },
  title: {
    color: "#fff",
    fontSize: 18,
    fontWeight: "700",
    marginBottom: 8
  },
  subtitle: {
    color: "#e5e7eb",
    fontSize: 14,
    marginBottom: 16
  },
  actions: {
    flexDirection: "row",
    justifyContent: "flex-end"
  },
  button: {
    paddingVertical: 12,
    paddingHorizontal: 18,
    borderRadius: 14
  },
  buttonText: {
    color: "#fff",
    fontWeight: "600"
  },
  accept: {
    backgroundColor: "#22c55e",
    marginRight: 12
  },
  reject: {
    backgroundColor: "#dc2626"
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
