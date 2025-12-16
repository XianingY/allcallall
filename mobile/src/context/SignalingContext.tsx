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
  mediaDevices as webrtcMediaDevices,
  RTCIceServer
} from "react-native-webrtc";

import { SignalingClient, SignalMessage } from "../api/signaling";
import { fetchWebRTCConfig } from "../api/webrtc";
import { useAuthContext } from "./AuthContext";
import { useSettings } from "./SettingsContext";
import AudioService from "../services/AudioServiceExpo";
import VibrationService from "../services/VibrationService";

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

const DEFAULT_ICE_SERVERS: RTCIceServer[] = [
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
  const { settings } = useSettings();
  const [status, setStatus] = useState<CallStatus>("idle");
  const [session, setSession] = useState<CallSession | null>(null);
  const [connectionReady, setConnectionReady] = useState(false);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [iceServers, setIceServers] = useState<RTCIceServer[]>(DEFAULT_ICE_SERVERS);

  const signalingRef = useRef<SignalingClient | null>(null);
  const peerRef = useRef<RTCPeerConnection | null>(null);
  const sessionRef = useRef<CallSession | null>(null);
  const pendingTarget = useRef<string | null>(null);
  const pendingLocalCandidates = useRef<IceCandidatePayload[]>([]);
  const pendingRemoteCandidates = useRef<IceCandidatePayload[]>([]);

  useEffect(() => {
    sessionRef.current = session;
  }, [session]);

  useEffect(() => {
    let cancelled = false;
    const loadIceServers = async () => {
      if (!token) {
        setIceServers(DEFAULT_ICE_SERVERS);
        return;
      }
      try {
        const config = await fetchWebRTCConfig(token);
        const servers = Array.isArray(config.ice_servers) ? config.ice_servers : [];
        if (!cancelled && servers.length) {
          setIceServers(servers as RTCIceServer[]);
          console.log("[SignalingContext] Using ICE servers from backend", servers);
        } else if (!cancelled) {
          setIceServers(DEFAULT_ICE_SERVERS);
        }
      } catch (error) {
        console.warn("[SignalingContext] Failed to load ICE servers, fallback to defaults", error);
        if (!cancelled) {
          setIceServers(DEFAULT_ICE_SERVERS);
        }
      }
    };
    loadIceServers();
    return () => {
      cancelled = true;
    };
  }, [token]);

  // 监听通话状态变化，播放相应的音频和震动提醒
  useEffect(() => {
    console.log("[SignalingContext] Status changed to:", status, "Session:", session?.direction);

    switch (status) {
      case "incoming":
        // 接到来电，播放来电铃声和震动
        console.log("[SignalingContext] Playing incoming call ringtone with vibration");
        if (settings.audioNotificationsEnabled) {
          AudioService.play("incoming_call");
        }
        if (settings.vibrationEnabled) {
          VibrationService.vibrate("incoming_call");
        }
        break;

      case "connecting":
        // 正在呼叫，播放回铃音和震动
        console.log("[SignalingContext] Connecting to remote peer, playing ringback");
        if (settings.audioNotificationsEnabled) {
          AudioService.play("ringback");
        }
        if (settings.vibrationEnabled) {
          VibrationService.vibrate("ringback");
        }
        break;

      case "in_call":
        // 通话接通，停止所有音频和震动
        console.log("[SignalingContext] Call connected, stopping all audio and vibration");
        AudioService.stopAll();
        VibrationService.cancel();
        if (settings.vibrationEnabled) {
          VibrationService.vibrate("call_connected"); // 通话接通提示音
        }
        break;

      case "idle":
        // 通话结束，停止所有音频和震动
        // 仅在从非idle状态改变到idle时才播放结束提示
        console.log("[SignalingContext] Call ended/idle, stopping all audio and vibration");
        AudioService.stopAll();
        VibrationService.cancel();
        // 只有当通话真实结束时才提示（由其他地方触发），避免应用启动时的误触
        break;
    }
  }, [status, session, settings.audioNotificationsEnabled, settings.vibrationEnabled]);

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
        Alert.alert("权限错误 / Permission Error", `无法获取权限: ${error instanceof Error ? error.message : String(error)} / Failed to get permissions.`);
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
        Alert.alert("错误 / Connection Issue", "信令服务未连接 / Signaling service not connected.");
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
        Alert.alert("错误 / Connection Issue", "无法发送信令消息 / Failed to send signaling message.");
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
      iceServers,
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
  }, [iceServers, resetCallState, sendMessage]);

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
        Alert.alert("错误 / Error", "请先登录 / Please log in first.");
        return;
      }
      
      if (status !== "idle") {
        console.warn("[startCall] Call already in progress. Current status:", status);
        Alert.alert("提示 / Tip", "已有通话在进行中，请先结束该通话 / A call is already in progress. Please end it first.");
        return;
      }

      try {
        console.log("[startCall] Requesting audio permissions...");
        const hasPermission = await ensureAudioPermission();
        if (!hasPermission) {
          console.warn("[startCall] Audio permission denied");
          Alert.alert("需要麦克风权限 / Microphone Permission Required", "请在系统设置中授予麦克风或蓝牙权限 / Please grant microphone or Bluetooth permission in system settings.");
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
          Alert.alert("错误 / Error", "请确认麦克风未被占用或已授权 / Please ensure the microphone is not in use or permissions are granted.");
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
      Alert.alert("需要麦克风权限 / Microphone Permission Required", "请在系统设置中授予麦克风权限 / Please grant microphone permission in system settings.");
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
        Alert.alert("呼叫错误 / Call Error", "无法解析对方的连接请求 / Failed to parse the remote call request.");
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
      Alert.alert("无法接通 / Failed to Answer", "请确认麦克风或蓝牙权限已授权 / Please ensure microphone or Bluetooth permissions are granted.");
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
