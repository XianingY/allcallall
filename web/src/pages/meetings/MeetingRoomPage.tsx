import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Circle, FileAudio, Mic, MicOff, PhoneOff, Radio, Users, Video, VideoOff, Volume2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { getRoom, startRecording, stopRecording } from "@/api/meetings";
import { PageError, PageLoading } from "@/components/PageState";
import { useMeetingEngine } from "@/meetings/useMeetingEngine";
import { useOrganization } from "@/organizations/OrganizationProvider";

export function MeetingRoomPage() {
  const roomId = Number(useParams().roomId); const navigate = useNavigate(); const queryClient = useQueryClient(); const { activeOrganization } = useOrganization();
  const options = useMemo(() => { try { return JSON.parse(sessionStorage.getItem(`meeting-options:${roomId}`) ?? "") as { audio: boolean; video: boolean; audioDeviceId?: string; videoDeviceId?: string }; } catch { return { audio: true, video: false }; } }, [roomId]);
  const meeting = useMeetingEngine(roomId, options); const room = useQuery({ queryKey: ["organizations", activeOrganization?.id, "rooms", roomId], queryFn: () => getRoom(roomId), enabled: Boolean(roomId), refetchInterval: 2500 });
  const localVideo = useRef<HTMLVideoElement>(null); const remoteVideo = useRef<HTMLVideoElement>(null); const [autoplayBlocked, setAutoplayBlocked] = useState(false);
  useEffect(() => { if (localVideo.current) localVideo.current.srcObject = meeting.localStream; }, [meeting.localStream]);
  useEffect(() => { if (remoteVideo.current) { remoteVideo.current.srcObject = meeting.remoteStream; void remoteVideo.current.play().catch(() => setAutoplayBlocked(true)); } }, [meeting.remoteStream]);
  const recording = useMutation({ mutationFn: () => room.data?.active_recording ? stopRecording(roomId) : startRecording(roomId), onSuccess: () => { void room.refetch(); void queryClient.invalidateQueries({ queryKey: ["organizations", activeOrganization?.id, "recordings"] }); } });
  if (room.isLoading) return <PageLoading label="正在加载会议" />; if (room.isError || !room.data) return <PageError error={room.error ?? new Error("会议不存在")} />;
  return <div className="meeting-room"><header><div><h1>{room.data.room.title}</h1><span><span className={`connection-dot ${meeting.state === "connected" ? "online" : ""}`} />{meeting.state}</span></div><div className="meeting-header-actions"><span><Users size={15} />{room.data.participant_count}/6</span>{room.data.latest_recording_id && <Link to={`/recordings/${room.data.latest_recording_id}`}><FileAudio size={16} />转写</Link>}</div></header>
    <main className="meeting-stage"><div className="remote-media"><video ref={remoteVideo} autoPlay playsInline />{!meeting.remoteStream && <div className="meeting-wait"><Users size={34} /><strong>等待其他参会人</strong><span>已加入 {room.data.participant_count} 人</span></div>}{autoplayBlocked && <button className="autoplay-button" onClick={() => { void remoteVideo.current?.play(); setAutoplayBlocked(false); }}><Volume2 size={17} />播放远端音频</button>}</div><div className="local-media"><video ref={localVideo} autoPlay muted playsInline />{!meeting.video && <VideoOff size={22} />}</div>{meeting.error && <div className="meeting-error">{meeting.error}</div>}</main>
    <aside className="participant-strip">{room.data.members.filter((member) => member.joined && !member.left).map((member) => <article key={member.id}><div className="participant-avatar">{(member.user_display_name || member.user_email || "?").slice(0, 1).toUpperCase()}</div><span>{member.user_display_name || member.user_email}</span>{member.audio_enabled ? <Mic size={13} /> : <MicOff size={13} />}</article>)}</aside>
    <footer className="meeting-controls"><button className={`meeting-control ${meeting.audio ? "" : "off"}`} aria-label={meeting.audio ? "静音" : "取消静音"} onClick={meeting.toggleAudio}>{meeting.audio ? <Mic size={19} /> : <MicOff size={19} />}</button><button className={`meeting-control ${meeting.video ? "" : "off"}`} aria-label={meeting.video ? "关闭摄像头" : "开启摄像头"} onClick={() => void meeting.toggleVideo()}>{meeting.video ? <Video size={19} /> : <VideoOff size={19} />}</button><button className={`record-control ${room.data.active_recording ? "recording" : ""}`} onClick={() => recording.mutate()}>{room.data.active_recording ? <><Circle size={14} fill="currentColor" />停止录制</> : <><Radio size={17} />开始录制</>}</button><button className="meeting-control leave" aria-label="离开会议" onClick={() => navigate("/meetings")}><PhoneOff size={19} /></button></footer>
  </div>;
}

