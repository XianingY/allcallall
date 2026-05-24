import React, { useEffect, useMemo, useRef } from "react";
import { Platform, View } from "react-native";

type StreamWithURL = MediaStream & {
  __allcallallStreamId?: string;
  toURL?: () => string;
};

const nativeRTC = Platform.OS === "web" ? null : require("react-native-webrtc");
const streamRegistry = new Map<string, MediaStream>();

const getStreamId = (stream: StreamWithURL) => {
  if (!stream.__allcallallStreamId) {
    stream.__allcallallStreamId = `stream-${Math.random().toString(36).slice(2)}`;
  }
  streamRegistry.set(stream.__allcallallStreamId, stream);
  return stream.__allcallallStreamId;
};

if (Platform.OS === "web" && typeof globalThis.MediaStream !== "undefined") {
  const prototype = globalThis.MediaStream.prototype as StreamWithURL;
  if (typeof prototype.toURL !== "function") {
    prototype.toURL = function toURL() {
      return getStreamId(this);
    };
  }
}

export type MediaStream = globalThis.MediaStream & { toURL?: () => string };
export type RTCPeerConnection = globalThis.RTCPeerConnection;
export type RTCIceCandidate = globalThis.RTCIceCandidate;
export type RTCSessionDescription = globalThis.RTCSessionDescription;

export const MediaStream = (nativeRTC?.MediaStream ?? globalThis.MediaStream) as {
  new (...args: any[]): MediaStream;
};
export const RTCPeerConnection = (nativeRTC?.RTCPeerConnection ??
  globalThis.RTCPeerConnection) as {
  new (...args: any[]): RTCPeerConnection;
};
export const RTCIceCandidate = (nativeRTC?.RTCIceCandidate ?? globalThis.RTCIceCandidate) as {
  new (...args: any[]): RTCIceCandidate;
};
export const RTCSessionDescription = (nativeRTC?.RTCSessionDescription ??
  globalThis.RTCSessionDescription) as {
  new (...args: any[]): RTCSessionDescription;
};

const webMediaDevices = {
  async getUserMedia(constraints: MediaStreamConstraints) {
    const stream = await navigator.mediaDevices.getUserMedia(constraints);
    getStreamId(stream as StreamWithURL);
    return stream as MediaStream;
  },
  enumerateDevices() {
    return navigator.mediaDevices.enumerateDevices();
  },
};

export const mediaDevices = (nativeRTC?.mediaDevices ?? webMediaDevices) as {
  getUserMedia(constraints: MediaStreamConstraints): Promise<MediaStream>;
  enumerateDevices?(): Promise<MediaDeviceInfo[]>;
};

type RTCViewProps = {
  streamURL: string;
  style?: any;
  objectFit?: "cover" | "contain";
  mirror?: boolean;
  zOrder?: number;
  pointerEvents?: string;
};

const WebRTCView: React.FC<RTCViewProps> = ({ streamURL, style, objectFit = "cover", mirror }) => {
  const ref = useRef<HTMLVideoElement | null>(null);
  const stream = useMemo(() => streamRegistry.get(streamURL) ?? null, [streamURL]);

  useEffect(() => {
    if (ref.current) {
      ref.current.srcObject = stream;
    }
  }, [stream]);

  return React.createElement("video", {
    ref,
    autoPlay: true,
    playsInline: true,
    muted: Boolean(mirror),
    style: {
      width: "100%",
      height: "100%",
      objectFit,
      transform: mirror ? "scaleX(-1)" : undefined,
      ...style,
    },
  });
};

const NativeRTCView = nativeRTC?.RTCView as React.ComponentType<RTCViewProps> | undefined;

export const RTCView: React.FC<RTCViewProps> = (props) => {
  if (Platform.OS === "web") {
    return <WebRTCView {...props} />;
  }
  if (!NativeRTCView) {
    return <View style={props.style} />;
  }
  return <NativeRTCView {...props} />;
};
