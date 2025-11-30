import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState
} from "react";
import {
  Alert,
  PermissionsAndroid,
  Platform
} from "react-native";
import {
  MediaStream,
  RTCPeerConnection,
  RTCIceCandidate,
  RTCSessionDescription,
  mediaDevices as webrtcMediaDevices
} from "react-native-webrtc";

import { SignalingClient, SignalMessage } from "../api/signaling";
import { useAuthContext } from "./AuthContext";

type CallDirection = "incoming" | "outgoing";

type SessionDescriptionPayload = RTCSessionDescriptionInit;

type IceCandidatePayload = RTCIceCandidateInit;

interface CallSession {
  callId: string;
  peerEmail: string;
  direction: CallDirection;
  offer?: SessionDescriptionPayload;
}

type CallStatus = "idle" | "connecting" | "incoming" | "in_call";

interface SignalingContextValue {
  status: CallStatus;
  session: CallSession | null;
  connectionReady: boolean;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  startCall: (email: string) => Promise<void>;
  acceptCall: () => Promise<void>;
  rejectCall: () => void;
  endCall: () => void;
}

const SignalingContext = createContext<SignalingContextValue | undefined>(
  undefined
);

const STUN_SERVERS = [
  { urls: "stun:stun.l.google.com:19302" },
  { urls: "stun:stun1.l.google.com:19302" },
  { urls: "stun:stun2.l.google.com:19302" },
  { urls: "stun:stun3.l.google.com:19302" },
  { urls: "stun:stun4.l.google.com:19302" }
];

const isSessionDescriptionPayload = (
  value: unknown
): value is SessionDescriptionPayload => {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { sdp?: unknown }).sdp === "string" &&
    typeof (value as { type?: unknown }).type === "string"
  );
};

const isIceCandidatePayload = (
  value: unknown
): value is IceCandidatePayload => {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { candidate?: unknown }).candidate === "string"
  );
};

export const SignalingProvider: React.FC<{ children: React.ReactNode }> = ({
  children
}) => {
  const { token, user } = useAuthContext();
  const [status, setStatus] = useState<CallStatus>("idle");
  const [session, setSession] = useState<CallSession | null>(null);
  const [connectionReady, setConnectionReady] = useState(false);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);

  const signalingRef = useRef<SignalingClient | null>(null);
  const peerRef = useRef<RTCPeerConnection | null>(null);
  const sessionRef = useRef<CallSession | null>(null);
  const pendingTarget = useRef<string | null>(null);
  const pendingLocalCandidates = useRef<IceCandidatePayload[]>([]);
  const pendingRemoteCandidates = useRef<IceCandidatePayload[]>([]);

  useEffect(() => {
    sessionRef.current = session;
  }, [session]);

  const ensureAudioPermission = useCallback(async () => {
    console.log("[ensureAudioPermission] Platform:", Platform.OS);
    
    if (Platform.OS === "android") {
      try {
        const permissions: string[] = [
          PermissionsAndroid.PERMISSIONS.RECORD_AUDIO
        ];

        if (Platform.Version >= 31) {
          permissions.push(PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT);
        }

        // 部分厂商在仅采集音频时也会检查摄像头权限，提前申请避免崩溃
        permissions.push(PermissionsAndroid.PERMISSIONS.CAMERA);

        console.log("[ensureAudioPermission] Requesting permissions:", permissions);
        
        // 直接请求权限，不使用超时（真机上应该正常工作）
        const result = await PermissionsAndroid.requestMultiple(permissions as any);
        console.log("[ensureAudioPermission] Permission result:", result);

        const allGranted = permissions.every(
          (permission) => (result as Record<string, any>)[permission] === PermissionsAndroid.RESULTS.GRANTED
        );
        
        console.log("[ensureAudioPermission] All permissions granted:", allGranted);
        return allGranted;
      } catch (error) {
        console.error("[ensureAudioPermission] Permission request error:", error);
        Alert.alert("权限错误", `无法获取权限: ${error instanceof Error ? error.message : String(error)}`);
        return false;
      }
    }
    console.log("[ensureAudioPermission] iOS platform, returning true");
    return true;
  }, []);

  const resetPeerResources = useCallback(() => {
    pendingLocalCandidates.current = [];
    pendingRemoteCandidates.current = [];

    if (peerRef.current) {
      (peerRef.current as any).onicecandidate = null;
      (peerRef.current as any).ontrack = null;
      (peerRef.current as any).onconnectionstatechange = null;
      peerRef.current.close();
      peerRef.current = null;
    }

    if (localStream) {
      localStream.getTracks().forEach((track) => track.stop());
    }
    if (remoteStream) {
      remoteStream.getTracks().forEach((track) => track.stop());
    }

    setLocalStream(null);
    setRemoteStream(null);
  }, [localStream, remoteStream]);

  const resetCallState = useCallback(() => {
    pendingTarget.current = null;
    setSession(null);
    sessionRef.current = null;
    setStatus("idle");
    resetPeerResources();
  }, [resetPeerResources]);

  const sendMessage = useCallback((message: SignalMessage) => {
    const client = signalingRef.current;
    console.log("[sendMessage] Attempting to send message:", message.type, "to:", message.to);
    
    if (!client) {
      console.warn("[sendMessage] No active signaling client, message dropped", message);
      if (message.type !== "ice.candidate") {
        Alert.alert("连接问题", "信令服务未连接。");
      }
      return;
    }
    
    try {
      console.log("[sendMessage] Sending message via client.send()...");
      const sent = client.send(message);
      if (!sent) {
        console.debug("[sendMessage] Signaling message queued until connection recovers", message.type);
      } else {
        console.log("[sendMessage] Message sent successfully");
      }
    } catch (error) {
      console.error("[sendMessage] Failed to send signaling message", error);
      if (message.type !== "ice.candidate") {
        Alert.alert("连接问题", "无法发送信令消息。");
      }
    }
  }, []);

  const enqueueRemoteCandidate = useCallback((candidate: IceCandidatePayload) => {
    const alreadyQueued = pendingRemoteCandidates.current.some(
      (item) =>
        item.candidate === candidate.candidate &&
        item.sdpMid === candidate.sdpMid &&
        item.sdpMLineIndex === candidate.sdpMLineIndex
    );
    if (!alreadyQueued) {
      pendingRemoteCandidates.current.push(candidate);
    }
  }, []);

  const flushPendingLocalCandidates = useCallback(
    (callId: string, peerEmail: string) => {
      if (!pendingLocalCandidates.current.length) {
        return;
      }
      const items = [...pendingLocalCandidates.current];
      pendingLocalCandidates.current = [];
      items.forEach((candidate) =>
        sendMessage({
          type: "ice.candidate",
          call_id: callId,
          to: peerEmail,
          payload: candidate
        })
      );
    },
    [sendMessage]
  );

  const drainRemoteCandidates = useCallback(async () => {
    const pc = peerRef.current;
    if (!pc || !pendingRemoteCandidates.current.length) {
      return;
    }
    const items = [...pendingRemoteCandidates.current];
    pendingRemoteCandidates.current = [];
    for (const candidate of items) {
      try {
        await pc.addIceCandidate(new RTCIceCandidate(candidate));
      } catch (error) {
        console.warn("Failed to add queued ICE candidate", error);
      }
    }
  }, []);

const createPeerConnection = useCallback(() => {
  const pc = new RTCPeerConnection({
    iceServers: STUN_SERVERS,
    bundlePolicy: "max-bundle",
    iceTransportPolicy: "all"
  } as any);

    (pc as any).onicecandidate = (event: any) => {
      if (!event.candidate) {
        return;
      }
      const candidateInit: IceCandidatePayload = {
        candidate: event.candidate.candidate,
        sdpMid: event.candidate.sdpMid ?? undefined,
        sdpMLineIndex: event.candidate.sdpMLineIndex ?? undefined
      };
      const current = sessionRef.current;
      if (current?.callId) {
        sendMessage({
          type: "ice.candidate",
          call_id: current.callId,
          to: current.peerEmail,
          payload: candidateInit as any
        });
      } else {
        pendingLocalCandidates.current.push(candidateInit);
      }
    };

    (pc as any).ontrack = (event: any) => {
      const [stream] = event.streams;
      if (stream) {
        setRemoteStream(stream);
      }
    };

    (pc as any).onconnectionstatechange = () => {
      if (
        pc.connectionState === "failed" ||
        pc.connectionState === "disconnected" ||
        pc.connectionState === "closed"
      ) {
        resetCallState();
      }
    };

    peerRef.current = pc;
    return pc;
  }, [resetCallState, sendMessage]);

  useEffect(() => {
    if (!token) {
      console.log("[SignalingContext] No token available, disconnecting");
      signalingRef.current?.disconnect();
      signalingRef.current = null;
      setConnectionReady(false);
      resetCallState();
      return;
    }

    console.log("[SignalingContext] Token available, initializing signaling client", {
      tokenLength: token.length,
      tokenPrefix: token.substring(0, 20) + "..."
    });
    const client = new SignalingClient(token);
    signalingRef.current = client;
    client.connect();

    const handleOpen = () => {
      console.log("[SignalingContext] Signaling connection opened successfully!");
      setConnectionReady(true);
    };
    const handleClose = () => {
      console.warn("[SignalingContext] Signaling connection closed");
      setConnectionReady(false);
      resetCallState();
    };

    const handleMessage = async (message: SignalMessage) => {
      console.log("[SignalingContext] Received message:", message.type, "from:", message.from);
      switch (message.type) {
        case "call.invite.ack":
          console.log("[SignalingContext] Received call.invite.ack, callId:", message.call_id, "pendingTarget:", pendingTarget.current);
          if (pendingTarget.current) {
            const newSession: CallSession = {
              callId: message.call_id ?? "",
              peerEmail: pendingTarget.current,
              direction: "outgoing"
            };
            console.log("[SignalingContext] Creating new session:", newSession);
            sessionRef.current = newSession;
            setSession(newSession);
            setStatus("connecting");
            if (newSession.callId) {
              console.log("[SignalingContext] Flushing pending local candidates");
              flushPendingLocalCandidates(
                newSession.callId,
                newSession.peerEmail
              );
            }
            pendingTarget.current = null;
          } else {
            console.warn("[SignalingContext] Received call.invite.ack but no pending target");
          }
          break;
        case "call.invite":
          if (!message.from || !isSessionDescriptionPayload(message.payload)) {
            Alert.alert("呼叫错误", "收到无效的呼叫请求");
            break;
          }
          setSession({
            callId: message.call_id ?? "",
            peerEmail: message.from,
            direction: "incoming",
            offer: message.payload as SessionDescriptionPayload
          });
          setStatus("incoming");
          break;
        case "call.accept":
          if (isSessionDescriptionPayload(message.payload)) {
            const pc = peerRef.current;
            if (pc && message.payload.sdp) {
              try {
                await pc.setRemoteDescription(
                  new RTCSessionDescription(message.payload as any)
                );
                await drainRemoteCandidates();
              } catch (error) {
                console.warn("Failed to apply remote answer", error);
              }
            }
          }
          setStatus("in_call");
          setSession((current) =>
            current
              ? {
                  ...current,
                  callId: message.call_id ?? current.callId
                }
              : current
          );
          if (sessionRef.current && message.call_id) {
            const current = {
              ...sessionRef.current,
              callId: message.call_id
            };
            sessionRef.current = current;
            flushPendingLocalCandidates(current.callId, current.peerEmail);
          }
          break;
        case "call.reject":
          Alert.alert("Call rejected", `${message.from} declined the call.`);
          resetCallState();
          break;
        case "call.end":
          Alert.alert("Call ended", `${message.from ?? "Peer"} ended the call.`);
          resetCallState();
          break;
        case "ice.candidate":
          if (isIceCandidatePayload(message.payload)) {
            const pc = peerRef.current;
            if (pc) {
              const hasRemoteDescription =
                pc.remoteDescription !== null &&
                typeof pc.remoteDescription?.type === "string";
              if (hasRemoteDescription) {
                try {
                  await pc.addIceCandidate(
                    new RTCIceCandidate(message.payload)
                  );
                } catch (error) {
                  console.warn("Failed to add ICE candidate", error);
                }
              } else {
                enqueueRemoteCandidate(message.payload);
              }
            } else {
              enqueueRemoteCandidate(message.payload);
            }
          }
          break;
        case "call.error":
          if (message.payload && typeof message.payload === "object" && "reason" in message.payload) {
            Alert.alert("Call error", String((message.payload as any).reason ?? "Error"));
          } else {
            Alert.alert("Call error", "Error");
          }
          resetCallState();
          break;
        default:
          break;
      }
    };

    client.on("open", handleOpen);
    client.on("close", handleClose);
    client.on("message", handleMessage);
    client.on("error", (err) => console.warn("signaling error", err));

    return () => {
      client.off("open", handleOpen);
      client.off("close", handleClose);
      client.off("message", handleMessage);
      client.disconnect();
      signalingRef.current = null;
    };
  }, [flushPendingLocalCandidates, resetCallState, drainRemoteCandidates, enqueueRemoteCandidate, token]);

  const startCall = useCallback(
    async (email: string) => {
      console.log("[startCall] Starting call to:", email, "Current status:", status);
      
      if (!user) {
        console.warn("[startCall] No user logged in");
        Alert.alert("错误", "请先登录");
        return;
      }
      
      if (status !== "idle") {
        console.warn("[startCall] Call already in progress. Current status:", status);
        Alert.alert("提示", "已有通话在进行中，请先结束该通话");
        return;
      }

      try {
        console.log("[startCall] Requesting audio permissions...");
        const hasPermission = await ensureAudioPermission();
        if (!hasPermission) {
          console.warn("[startCall] Audio permission denied");
          Alert.alert("需要麦克风权限", "请在系统设置中授予麦克风或蓝牙权限。");
          return;
        }
        console.log("[startCall] Audio permission granted");

        console.log("[startCall] Resetting peer resources...");
        resetPeerResources();
        
        console.log("[startCall] Requesting media stream...");
        console.log("[startCall] webrtcMediaDevices:", webrtcMediaDevices ? "available" : "null");
        
        if (!webrtcMediaDevices) {
          throw new Error("WebRTC mediaDevices not available. Please use 'expo run:android' to build a native app.");
        }
        
        console.log("[startCall] Requesting getUserMedia with audio only...");
        const stream = await webrtcMediaDevices.getUserMedia({
          audio: true,
          video: false
        });
        console.log("[startCall] Media stream obtained:", stream.getTracks().length, "tracks");
        stream.getTracks().forEach((track) => {
          console.log("[startCall] Track obtained - Kind:", track.kind, "Enabled:", track.enabled);
        });
        setLocalStream(stream);

        console.log("[startCall] Creating peer connection...");
        const pc = createPeerConnection();
        stream.getTracks().forEach((track) => {
          console.log("[startCall] Adding track:", track.kind);
          pc.addTrack(track, stream);
        });

        console.log("[startCall] Creating offer...");
        const offer = await pc.createOffer({
          offerToReceiveAudio: true,
          offerToReceiveVideo: false
        });
        console.log("[startCall] Offer created, SDP length:", offer.sdp?.length);
        
        console.log("[startCall] Setting local description...");
        await pc.setLocalDescription(offer);
        console.log("[startCall] Local description set");

        pendingTarget.current = email;
        setStatus("connecting");
        console.log("[startCall] Status changed to 'connecting'");
        
        console.log("[startCall] Sending call.invite message...");
        sendMessage({
          type: "call.invite",
          to: email,
          payload: {
            sdp: offer.sdp,
            type: offer.type
          }
        });
        console.log("[startCall] call.invite message sent");
      } catch (error) {
        console.error("[startCall] Error occurred:", error);
        console.error("[startCall] Error name:", (error as Error)?.name);
        console.error("[startCall] Error message:", (error as Error)?.message);
        const errorMsg = error instanceof Error ? error.message : String(error);
        Alert.alert("无法发起通话", `错误: ${errorMsg}\n请确认麦克风未被占用或已授权。`);
        resetPeerResources();
        setStatus("idle");
      }
    },
    [createPeerConnection, ensureAudioPermission, resetPeerResources, sendMessage, status, user]
  );

  const acceptCall = useCallback(async () => {
    if (!session || session.direction !== "incoming" || !session.offer) {
      return;
    }

    const hasPermission = await ensureAudioPermission();
    if (!hasPermission) {
      Alert.alert("需要麦克风权限", "请在系统设置中授予麦克风权限。");
      return;
    }

    try {
      console.log("[acceptCall] Requesting media stream...");
      console.log("[acceptCall] webrtcMediaDevices:", webrtcMediaDevices ? "available" : "null");
      
      if (!webrtcMediaDevices) {
        throw new Error("WebRTC mediaDevices not available. Please use 'expo run:android' to build a native app.");
      }
      
      console.log("[acceptCall] Requesting getUserMedia with audio only...");
      const stream = await webrtcMediaDevices.getUserMedia({
        audio: true,
        video: false
      });
      console.log("[acceptCall] Media stream obtained:", stream.getTracks().length, "tracks");
      stream.getTracks().forEach((track) => {
        console.log("[acceptCall] Track obtained - Kind:", track.kind, "Enabled:", track.enabled);
      });
      setLocalStream(stream);

      const pc = createPeerConnection();
      stream.getTracks().forEach((track) => pc.addTrack(track, stream));

      try {
        await pc.setRemoteDescription(new RTCSessionDescription(session.offer as any));
      } catch (error) {
        console.warn("setRemoteDescription failed", error);
        Alert.alert("呼叫错误", "无法解析对方的连接请求");
        resetCallState();
        return;
      }

      const answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
      await drainRemoteCandidates();

      sendMessage({
        type: "call.accept",
        call_id: session.callId,
        to: session.peerEmail,
        payload: {
          sdp: answer.sdp,
          type: answer.type
        }
      });

      setStatus("in_call");
    } catch (error) {
      console.error("acceptCall error", error);
      Alert.alert("无法接通", "请确认麦克风或蓝牙权限已授权。");
      resetCallState();
    }
  }, [createPeerConnection, drainRemoteCandidates, ensureAudioPermission, resetCallState, sendMessage, session]);

  const rejectCall = useCallback(() => {
    if (!session) {
      return;
    }
    sendMessage({
      type: "call.reject",
      call_id: session.callId,
      to: session.peerEmail
    });
    resetCallState();
  }, [resetCallState, sendMessage, session]);

  const endCall = useCallback(() => {
    if (!session) {
      return;
    }
    sendMessage({
      type: "call.end",
      call_id: session.callId,
      to: session.peerEmail
    });
    resetCallState();
  }, [resetCallState, sendMessage, session]);

  const value = useMemo<SignalingContextValue>(
    () => ({
      status,
      session,
      connectionReady,
      localStream,
      remoteStream,
      startCall,
      acceptCall,
      rejectCall,
      endCall
    }),
    [
      status,
      session,
      connectionReady,
      localStream,
      remoteStream,
      startCall,
      acceptCall,
      rejectCall,
      endCall
    ]
  );

  return (
    <SignalingContext.Provider value={value}>
      {children}
    </SignalingContext.Provider>
  );
};

export const useSignaling = () => {
  const ctx = useContext(SignalingContext);
  if (!ctx) {
    throw new Error("useSignaling must be used within SignalingProvider");
  }
  return ctx;
};
