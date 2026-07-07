import { useRef, useEffect, useCallback, useState } from 'react';
import {
  RTCPeerConnection,
  RTCSessionDescription,
  RTCIceCandidate,
  MediaStream,
} from 'react-native-webrtc';

interface WebRTCConfig {
  iceServers: any[];
  onOfferCreated: (offer: any) => void;
  onAnswerCreated: (answer: any) => void;
  onIceCandidate: (candidate: any) => void;
  onRemoteStream?: (stream: MediaStream) => void;
  onConnectionStateChange?: (state: string) => void;
}

interface WebRTCHook {
  peerConnection: RTCPeerConnection | null;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  connectionState: string;
  createOffer: () => Promise<void>;
  createAnswer: (offer: any) => Promise<void>;
  setRemoteDescription: (desc: any) => Promise<void>;
  addIceCandidate: (candidate: any) => Promise<void>;
  addLocalStream: (stream: MediaStream) => void;
  removeLocalStream: () => void;
  close: () => void;
}

export function useWebRTC(config: WebRTCConfig): WebRTCHook {
  const peerConnectionRef = useRef<RTCPeerConnection | null>(null);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [connectionState, setConnectionState] = useState<string>('new');

  // Initialize peer connection
  useEffect(() => {
    const pc = new RTCPeerConnection({
      iceServers: config.iceServers,
    } as any);

    // Handle ICE candidates
    pc.addEventListener('icecandidate', (event: any) => {
      if (event.candidate) {
        config.onIceCandidate(event.candidate);
      }
    });

    // Handle remote stream
    pc.addEventListener('track', (event: any) => {
      if (event.streams && event.streams[0]) {
        setRemoteStream(event.streams[0]);
        config.onRemoteStream?.(event.streams[0]);
      }
    });

    // Handle connection state changes
    pc.addEventListener('connectionstatechange', () => {
      setConnectionState(pc.connectionState);
      config.onConnectionStateChange?.(pc.connectionState);
    });

    peerConnectionRef.current = pc;

    return () => {
      pc.close();
      peerConnectionRef.current = null;
    };
  }, [config.iceServers, config.onIceCandidate, config.onRemoteStream, config.onConnectionStateChange]);

  const createOffer = useCallback(async () => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    config.onOfferCreated(offer);
  }, [config.onOfferCreated]);

  const createAnswer = useCallback(async (offer: any) => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    await pc.setRemoteDescription(offer);
    const answer = await pc.createAnswer();
    await pc.setLocalDescription(answer);
    config.onAnswerCreated(answer);
  }, [config.onAnswerCreated]);

  const setRemoteDescription = useCallback(async (desc: any) => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    await pc.setRemoteDescription(desc);
  }, []);

  const addIceCandidate = useCallback(async (candidate: any) => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    await pc.addIceCandidate(candidate);
  }, []);

  const addLocalStream = useCallback((stream: MediaStream) => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    stream.getTracks().forEach((track: any) => {
      pc.addTrack(track, stream);
    });
    setLocalStream(stream);
  }, []);

  const removeLocalStream = useCallback(() => {
    const pc = peerConnectionRef.current;
    if (!pc || !localStream) return;

    localStream.getTracks().forEach((track: any) => {
      track.stop();
      const senders = pc.getSenders();
      const sender = senders.find((s: any) => s.track === track);
      if (sender) {
        pc.removeTrack(sender);
      }
    });
    setLocalStream(null);
  }, [localStream]);

  const close = useCallback(() => {
    const pc = peerConnectionRef.current;
    if (pc) {
      pc.close();
      peerConnectionRef.current = null;
    }
    setLocalStream(null);
    setRemoteStream(null);
    setConnectionState('closed');
  }, []);

  return {
    peerConnection: peerConnectionRef.current,
    localStream,
    remoteStream,
    connectionState,
    createOffer,
    createAnswer,
    setRemoteDescription,
    addIceCandidate,
    addLocalStream,
    removeLocalStream,
    close,
  };
}
