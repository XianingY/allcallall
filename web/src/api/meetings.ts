import type { components } from "@/api/schema";
import { apiDownload, apiRequest } from "@/api/http";

export type Room = components["schemas"]["Room"];
export type RoomMember = components["schemas"]["RoomMember"];
export type Recording = components["schemas"]["Recording"];
export type RecordingTranscription = components["schemas"]["RecordingTranscription"];
export type TranscriptSegment = components["schemas"]["MeetingTranscriptSegment"];
export type TranscriptPage = components["schemas"]["RecordingTranscriptPage"];

export const listRooms = () => apiRequest<{ rooms: Room[] }>("/rooms").then((value) => value.rooms);
export const createRoom = (input: { title: string; participant_ids?: number[]; conversation_id?: number }) => apiRequest<{ room: Room }>("/rooms", { method: "POST", body: JSON.stringify(input) }).then((value) => value.room);
export const getRoom = (id: number) => apiRequest<{ room: Room }>(`/rooms/${id}/state`).then((value) => value.room);
export const joinRoom = (id: number) => apiRequest<{ room: Room }>(`/rooms/${id}/join`, { method: "POST" }).then((value) => value.room);
export const leaveRoom = (id: number) => apiRequest<{ room: Room }>(`/rooms/${id}/leave`, { method: "POST" }).then((value) => value.room);
export const sendRoomOffer = (id: number, sdp: string) => apiRequest<{ room: Room; answer: RTCSessionDescriptionInit }>(`/rooms/${id}/offer`, { method: "POST", body: JSON.stringify({ sdp }) });
export const sendRoomICE = (id: number, candidate: RTCIceCandidateInit) => apiRequest<void>(`/rooms/${id}/ice`, { method: "POST", body: JSON.stringify(candidate) });
export const updateRoomMedia = (id: number, input: { audio_enabled?: boolean; video_enabled?: boolean; connection_state?: string }) => apiRequest<void>(`/rooms/${id}/media`, { method: "POST", body: JSON.stringify(input) });
export const startRecording = (id: number) => apiRequest<{ recording: Recording }>(`/rooms/${id}/recording/start`, { method: "POST" }).then((value) => value.recording);
export const stopRecording = (id: number) => apiRequest<{ recording: Recording }>(`/rooms/${id}/recording/stop`, { method: "POST" }).then((value) => value.recording);

export const listRecordings = () => apiRequest<{ recordings: Recording[] }>("/recordings").then((value) => value.recordings);
export const getRecording = (id: number) => apiRequest<{ recording: Recording }>(`/recordings/${id}`).then((value) => value.recording);
export const getTranscript = (id: number, afterId?: number) => apiRequest<TranscriptPage>(`/recordings/${id}/transcript?limit=100${afterId ? `&after_id=${afterId}` : ""}`);
export const retryTranscription = (id: number) => apiRequest<{ transcription: RecordingTranscription }>(`/recordings/${id}/transcription/retry`, { method: "POST" }).then((value) => value.transcription);
export const downloadRecording = (recordingId: number, fileId: number) => apiDownload(`/recordings/${recordingId}/files/${fileId}`);
