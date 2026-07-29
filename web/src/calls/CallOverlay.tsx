import { Mic, MicOff, Phone, PhoneOff, Video, VideoOff, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { useCall } from "@/calls/CallContext";
import { useCallStore } from "@/calls/callStore";

export function CallOverlay() {
  const state = useCallStore(); const call = useCall(); const localVideo = useRef<HTMLVideoElement>(null); const remoteVideo = useRef<HTMLVideoElement>(null); const [inputs, setInputs] = useState<MediaDeviceInfo[]>([]);
  useEffect(() => { if (localVideo.current) localVideo.current.srcObject = state.localStream; }, [state.localStream]);
  useEffect(() => { if (remoteVideo.current) remoteVideo.current.srcObject = state.remoteStream; }, [state.remoteStream]);
  useEffect(() => { if (state.status !== "idle") void navigator.mediaDevices?.enumerateDevices().then((items) => setInputs(items.filter((item) => item.kind === "audioinput"))).catch((err) => console.error("[CallOverlay] enumerateDevices failed", err)); }, [state.status]);
  if (state.status === "idle") return null;
  const incoming = state.status === "incoming";
  return <div className="call-overlay" role="dialog" aria-label="浏览器通话"><div className="call-window">
    <header><div><span>{incoming ? "来电" : state.status === "connected" ? "通话中" : state.status === "reconnecting" ? "正在恢复连接" : "正在连接"}</span><strong>{state.peerEmail || "未知联系人"}</strong></div><button className="icon-button" aria-label="关闭通话" onClick={call.end}><X size={19} /></button></header>
    <div className="call-stage"><video ref={remoteVideo} autoPlay playsInline /><video className="local-video" ref={localVideo} autoPlay playsInline muted />{!state.remoteStream && <div className="call-avatar">{state.peerEmail.slice(0, 1).toUpperCase()}</div>}{state.error && <p className="call-error">{state.error}</p>}</div>
    {incoming ? <footer><button className="call-action reject" aria-label="拒绝" onClick={call.reject}><PhoneOff size={20} /></button><button className="call-action accept" aria-label="接听" onClick={() => void call.accept()}><Phone size={20} /></button></footer> : <footer><button className="call-action" aria-label={state.muted ? "取消静音" : "静音"} onClick={call.toggleMute}>{state.muted ? <MicOff size={20} /> : <Mic size={20} />}</button><button className="call-action" aria-label={state.cameraEnabled ? "关闭摄像头" : "开启摄像头"} onClick={() => void call.toggleCamera()}>{state.cameraEnabled ? <Video size={20} /> : <VideoOff size={20} />}</button><select aria-label="麦克风设备" onChange={(event) => void call.switchInput(event.target.value)}><option value="">默认麦克风</option>{inputs.map((input) => <option key={input.deviceId} value={input.deviceId}>{input.label || `麦克风 ${inputs.indexOf(input) + 1}`}</option>)}</select><button className="call-action reject" aria-label="挂断" onClick={call.end}><PhoneOff size={20} /></button></footer>}
  </div></div>;
}

