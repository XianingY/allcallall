import { RTCPeerConnection } from "../../platform/rtc";

export interface PeerConnectionHandlers {
  onIceCandidate: (candidate: any) => void;
  onIceConnectionStateChange: () => void;
  onTrack: (event: any) => void;
  onDataChannel: (event: any) => void;
  onConnectionStateChange: () => void;
}

/**
 * Creates an RTCPeerConnection and wires the signaling-relevant event handlers.
 * The handler *behaviour* is supplied by the caller (SignalingProvider) so the
 * factory stays free of React/producer state — this is the single extraction
 * point for the raw WebRTC object.
 */
export const createSignalingPeerConnection = (
  iceServers: RTCIceServer[],
  handlers: PeerConnectionHandlers
): RTCPeerConnection => {
  const pc = new RTCPeerConnection({ iceServers, bundlePolicy: "max-bundle" });
  pc.onicecandidate = (event: any) => handlers.onIceCandidate(event.candidate);
  pc.oniceconnectionstatechange = () => handlers.onIceConnectionStateChange();
  pc.ontrack = (event: any) => handlers.onTrack(event);
  pc.ondatachannel = (event: any) => handlers.onDataChannel(event);
  pc.onconnectionstatechange = () => handlers.onConnectionStateChange();
  return pc;
};
