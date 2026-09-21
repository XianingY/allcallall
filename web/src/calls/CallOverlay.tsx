import { Mic, MicOff, Phone, PhoneOff, Video, VideoOff, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { useTranslation } from "react-i18next";

import { useCall } from "@/calls/CallContext";
import { useCallStore } from "@/calls/callStore";

export function CallOverlay() {
  const { t } = useTranslation(); const state = useCallStore(); const call = useCall(); const localVideo = useRef<HTMLVideoElement>(null); const remoteVideo = useRef<HTMLVideoElement>(null); const [inputs, setInputs] = useState<MediaDeviceInfo[]>([]);
  useEffect(() => { if (localVideo.current) localVideo.current.srcObject = state.localStream; }, [state.localStream]);
  useEffect(() => { if (remoteVideo.current) remoteVideo.current.srcObject = state.remoteStream; }, [state.remoteStream]);
  useEffect(() => { if (state.status !== "idle") void navigator.mediaDevices?.enumerateDevices().then((items) => setInputs(items.filter((item) => item.kind === "audioinput"))).catch((err) => console.error("[CallOverlay] enumerateDevices failed", err)); }, [state.status]);
  if (state.status === "idle") return null;
  const incoming = state.status === "incoming";
  return <div className="call-overlay" role="dialog" aria-label={t("call.title")}><div className="call-window">
    <header><div><span>{incoming ? t("call.incoming") : state.status === "connected" ? t("call.connected") : state.status === "reconnecting" ? t("call.reconnecting") : t("call.connecting")}</span><strong>{state.peerEmail || t("call.unknownContact")}</strong></div><button className="icon-button" aria-label={t("call.close")} onClick={call.end}><X size={19} /></button></header>
    <div className="call-stage"><video ref={remoteVideo} autoPlay playsInline /><video className="local-video" ref={localVideo} autoPlay playsInline muted />{!state.remoteStream && <div className="call-avatar">{state.peerEmail.slice(0, 1).toUpperCase()}</div>}{state.error && <p className="call-error">{state.error}</p>}</div>
    {incoming ? <footer><button className="call-action reject" aria-label={t("call.reject")} onClick={call.reject}><PhoneOff size={20} /></button><button className="call-action accept" aria-label={t("call.accept")} onClick={() => void call.accept()}><Phone size={20} /></button></footer> : <footer><button className="call-action" aria-label={state.muted ? t("call.unmute") : t("call.mute")} onClick={call.toggleMute}>{state.muted ? <MicOff size={20} /> : <Mic size={20} />}</button><button className="call-action" aria-label={state.cameraEnabled ? t("call.cameraOff") : t("call.cameraOn")} onClick={() => void call.toggleCamera()}>{state.cameraEnabled ? <Video size={20} /> : <VideoOff size={20} />}</button><select aria-label={t("call.micDevice")} onChange={(event) => void call.switchInput(event.target.value)}><option value="">{t("call.micDefault")}</option>{inputs.map((input) => <option key={input.deviceId} value={input.deviceId}>{input.label || t("call.micFallback", { index: inputs.indexOf(input) + 1 })}</option>)}</select><button className="call-action reject" aria-label={t("call.hangup")} onClick={call.end}><PhoneOff size={20} /></button></footer>}
  </div></div>;
}

