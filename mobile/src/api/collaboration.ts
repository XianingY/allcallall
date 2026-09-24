// #22：/conversations、/deals、/rooms、/recordings、/pipelines、/organizations/{id}/policy
// 已纳入 openapi.yaml，记录类型改为引用 @allcallall/api-types 生成的共享契约。
import { createApiClient, getActiveOrganizationHeader } from "./client";
import type { components } from "@allcallall/api-types";
import { API_BASE_URL } from "../config";

type APISchemas = components["schemas"];

export type OrganizationRecord = APISchemas["Organization"];

export type OrganizationPolicyRecord = APISchemas["OrganizationPolicy"];

export type ConversationRecord = APISchemas["Conversation"];

export type ConversationNoteRecord = APISchemas["ConversationNote"];

export type ConversationFollowupRecord = APISchemas["ConversationFollowup"];

export type MeetingSummaryCard = APISchemas["MeetingSummary"];

export interface ConversationWorkspaceRecord {
  latest_meeting?: RoomListItemRecord | null;
  latest_recording?: RecordingRecord | null;
  meeting_summary?: MeetingSummaryCard | null;
  latest_note?: ConversationNoteRecord | null;
  agent_context: {
    latest_call_id?: string;
    transcript_segment_count: number;
    latest_transcript_at?: string | null;
    meeting_transcription_status?: string;
    meeting_transcription_error?: string;
    meeting_transcript_segment_count?: number;
    latest_meeting_transcript_at?: string | null;
    latest_memory_keys?: string[];
    last_agent_run_at?: string | null;
    last_agent_status?: string;
    last_workflow_id?: number | null;
    last_workflow_preset?: string;
    pending_approval_count?: number;
    knowledge_source_count?: number;
  };
  assignee_user_id?: number | null;
  assignee_label?: string;
  status: string;
  priority: string;
}

export type ConversationDetailRecord = APISchemas["ConversationDetail"];

export type MessageRecord = APISchemas["Message"];

export type RoomMemberRecord = APISchemas["RoomMember"];

export type RoomEventRecord = APISchemas["RoomEvent"];

export type RecordingSessionRecord = APISchemas["RecordingSession"];

export type RecordingFileRecord = APISchemas["RecordingFile"];

export type RecordingTranscriptionRecord = APISchemas["RecordingTranscription"];

export type RecordingRecord = APISchemas["Recording"];

export type MeetingTranscriptSegmentRecord = APISchemas["MeetingTranscriptSegment"];

export type RecordingTranscriptPage = APISchemas["RecordingTranscriptPage"];

export type RoomRecord = APISchemas["Room"];

export type RoomListItemRecord = APISchemas["RoomListItem"];

export type RoomOfferAnswer = APISchemas["RoomOfferAnswer"];

export interface MeetingJoinOptions {
  audioEnabled: boolean;
  videoEnabled: boolean;
  cameraFacing: "front" | "back";
  speakerOn: boolean;
}

export interface MeetingParticipantView {
  user_id: number;
  display_name: string;
  email: string;
  is_host: boolean;
  connection_state: string;
  audio_enabled: boolean;
  video_enabled: boolean;
}

export interface MeetingDeviceState {
  audioEnabled: boolean;
  videoEnabled: boolean;
  speakerOn: boolean;
  cameraFacing: "front" | "back";
}

export interface MeetingControlState {
  joined: boolean;
  joining: boolean;
  connectionState:
    | "idle"
    | "connecting"
    | "connected"
    | "reconnecting"
    | "failed";
}

export type PipelineStageRecord = APISchemas["PipelineStage"];

export type PipelineRecord = APISchemas["Pipeline"];

export type DealRecord = APISchemas["Deal"];

export type DealActivityRecord = APISchemas["DealActivity"];

export const listOrganizations = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ organizations: OrganizationRecord[] }>(
    "/organizations",
  );
  return response.data.organizations;
};

export const createOrganization = async (token: string, name: string) => {
  const api = createApiClient(token);
  const response = await api.post<{ organization: OrganizationRecord }>(
    "/organizations",
    { name },
  );
  return response.data.organization;
};

export const switchOrganization = async (
  token: string,
  organizationId: number,
) => {
  const api = createApiClient(token);
  const response = await api.post<{ organization: OrganizationRecord }>(
    `/organizations/${organizationId}/switch`,
  );
  return response.data.organization;
};

export const fetchOrganizationPolicy = async (
  token: string,
  organizationId: number,
) => {
  const api = createApiClient(token);
  const response = await api.get<{ policy: OrganizationPolicyRecord }>(
    `/organizations/${organizationId}/policy`,
  );
  return response.data.policy;
};

export const updateOrganizationPolicy = async (
  token: string,
  organizationId: number,
  payload: Pick<
    OrganizationPolicyRecord,
    "recording_mode" | "recording_storage_days" | "recording_export_allowed"
  >,
) => {
  const api = createApiClient(token);
  const response = await api.put<{ policy: OrganizationPolicyRecord }>(
    `/organizations/${organizationId}/policy`,
    payload,
  );
  return response.data.policy;
};

export interface PageRequest {
  limit?: number;
  offset?: number;
}

export type Pagination = APISchemas["Pagination"];

export interface ConversationPage {
  conversations: ConversationRecord[];
  pagination: Pagination;
}

export type DealPage = APISchemas["DealPage"];

export const listConversations = async (
  token: string,
  filter?: string,
  contactId?: number,
  page?: PageRequest,
) => {
  const api = createApiClient(token);
  const response = await api.get<ConversationPage>(
    "/conversations",
    {
      params: {
        ...(filter ? { filter } : {}),
        ...(contactId ? { contact_id: contactId } : {}),
        ...(page?.limit !== undefined ? { limit: page.limit } : {}),
        ...(page?.offset !== undefined ? { offset: page.offset } : {}),
      },
    },
  );
  return response.data;
};

export const fetchConversationDetail = async (
  token: string,
  conversationId: number,
) => {
  const api = createApiClient(token);
  const response = await api.get<{ conversation: ConversationDetailRecord }>(
    `/conversations/${conversationId}`,
  );
  return response.data.conversation;
};

export type CreateConversationPayload = APISchemas["CreateConversationRequest"];

export const createConversation = async (
  token: string,
  payload: CreateConversationPayload,
) => {
  const api = createApiClient(token);
  const response = await api.post<{ conversation: ConversationRecord }>(
    "/conversations",
    payload,
  );
  return response.data.conversation;
};

export type UpdateConversationPayload = APISchemas["UpdateConversationRequest"];

export const updateConversation = async (
  token: string,
  conversationId: number,
  payload: UpdateConversationPayload,
) => {
  const api = createApiClient(token);
  const response = await api.patch<{ conversation: ConversationRecord }>(
    `/conversations/${conversationId}`,
    payload,
  );
  return response.data.conversation;
};

export type CreateRoomPayload = APISchemas["CreateRoomRequest"];

export const listRooms = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ rooms: RoomRecord[] }>("/rooms");
  return response.data.rooms;
};

export const createRoom = async (token: string, payload: CreateRoomPayload) => {
  const api = createApiClient(token);
  const response = await api.post<{ room: RoomRecord }>("/rooms", payload);
  return response.data.room;
};

export const createConversationRoom = async (
  token: string,
  conversationId: number,
  title?: string,
) => {
  const api = createApiClient(token);
  const response = await api.post<{ room: RoomRecord }>(
    `/conversations/${conversationId}/rooms`,
    title ? { title } : {},
  );
  return response.data.room;
};

export const fetchRoomState = async (token: string, roomId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ room: RoomRecord }>(
    `/rooms/${roomId}/state`,
  );
  return response.data.room;
};

export const joinRoom = async (token: string, roomId: number) => {
  const api = createApiClient(token);
  const response = await api.post<{ room: RoomRecord }>(
    `/rooms/${roomId}/join`,
  );
  return response.data.room;
};

export const sendRoomOffer = async (
  token: string,
  roomId: number,
  sdp: string,
) => {
  const api = createApiClient(token);
  const response = await api.post<{
    room: RoomRecord;
    answer: RoomOfferAnswer;
  }>(`/rooms/${roomId}/offer`, { sdp });
  return response.data;
};

export type RoomIceCandidatePayload = APISchemas["RoomIceCandidateRequest"];

export type RoomMediaStatePayload = APISchemas["RoomMediaStateRequest"];

export const addRoomIceCandidate = async (
  token: string,
  roomId: number,
  payload: RoomIceCandidatePayload,
) => {
  const api = createApiClient(token);
  await api.post(`/rooms/${roomId}/ice`, payload);
};

export const updateRoomMediaState = async (
  token: string,
  roomId: number,
  payload: RoomMediaStatePayload,
) => {
  const api = createApiClient(token);
  await api.post(`/rooms/${roomId}/media`, payload);
};

export const leaveRoom = async (token: string, roomId: number) => {
  const api = createApiClient(token);
  const response = await api.post<{ room: RoomRecord }>(
    `/rooms/${roomId}/leave`,
  );
  return response.data.room;
};

export const startRoomRecording = async (token: string, roomId: number) => {
  const api = createApiClient(token);
  const response = await api.post<{ recording: RecordingRecord }>(
    `/rooms/${roomId}/recording/start`,
  );
  return response.data.recording;
};

export const stopRoomRecording = async (token: string, roomId: number) => {
  const api = createApiClient(token);
  const response = await api.post<{ recording: RecordingRecord }>(
    `/rooms/${roomId}/recording/stop`,
  );
  return response.data.recording;
};

export type RecordingPage = APISchemas["RecordingPage"];

export const listRecordings = async (token: string, page?: PageRequest) => {
  const api = createApiClient(token);
  const response = await api.get<RecordingPage>("/recordings", {
    params: {
      ...(page?.limit !== undefined ? { limit: page.limit } : {}),
      ...(page?.offset !== undefined ? { offset: page.offset } : {}),
    },
  });
  return response.data;
};

export const fetchRecording = async (token: string, recordingId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ recording: RecordingRecord }>(
    `/recordings/${recordingId}`,
  );
  return response.data.recording;
};

export const fetchRecordingTranscript = async (
  token: string,
  recordingId: number,
  params?: { afterId?: number; limit?: number },
): Promise<RecordingTranscriptPage> => {
  const api = createApiClient(token);
  const response = await api.get<RecordingTranscriptPage>(
    `/recordings/${recordingId}/transcript`,
    {
      params: {
        after_id: params?.afterId,
        limit: params?.limit ?? 100,
      },
    },
  );
  return response.data;
};

export const retryRecordingTranscription = async (
  token: string,
  recordingId: number,
): Promise<RecordingTranscriptionRecord> => {
  const api = createApiClient(token);
  const response = await api.post<{ transcription: RecordingTranscriptionRecord }>(
    `/recordings/${recordingId}/transcription/retry`,
  );
  return response.data.transcription;
};

export const buildRecordingDownloadRequest = (
  token: string,
  recordingId: number,
  fileId: number,
) => {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
    Accept: "*/*",
  };
  const organizationId = getActiveOrganizationHeader();
  if (organizationId) {
    headers["X-Organization-ID"] = String(organizationId);
  }
  return {
    fromUrl: `${API_BASE_URL}/recordings/${recordingId}/files/${fileId}`,
    headers,
  };
};

export type MessagePage = APISchemas["MessagePage"];

export const listMessages = async (
  token: string,
  conversationId: number,
  cursor?: { beforeId?: number; afterId?: number; limit?: number },
) => {
  const api = createApiClient(token);
  const response = await api.get<MessagePage>(
    `/conversations/${conversationId}/messages`,
    {
      params: {
        ...(cursor?.beforeId !== undefined ? { before_id: cursor.beforeId } : {}),
        ...(cursor?.afterId !== undefined ? { after_id: cursor.afterId } : {}),
        ...(cursor?.limit !== undefined ? { limit: cursor.limit } : {}),
      },
    },
  );
  return response.data;
};

export type CreateMessagePayload = APISchemas["CreateMessageRequest"];

export const createMessage = async (
  token: string,
  conversationId: number,
  payload: CreateMessagePayload,
) => {
  const api = createApiClient(token);
  const response = await api.post<{ message: MessageRecord }>(
    `/conversations/${conversationId}/messages`,
    payload,
  );
  return response.data.message;
};

export const markConversationRead = async (
  token: string,
  conversationId: number,
) => {
  const api = createApiClient(token);
  await api.post(`/conversations/${conversationId}/read`);
};

export const listConversationNotes = async (
  token: string,
  conversationId: number,
) => {
  const api = createApiClient(token);
  const response = await api.get<{ notes: ConversationNoteRecord[] }>(
    `/conversations/${conversationId}/notes`,
  );
  return response.data.notes;
};

export const createConversationNote = async (
  token: string,
  conversationId: number,
  body: string,
) => {
  const api = createApiClient(token);
  const response = await api.post<{ note: ConversationNoteRecord }>(
    `/conversations/${conversationId}/notes`,
    { body },
  );
  return response.data.note;
};

export const listPipelines = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ pipelines: PipelineRecord[] }>("/pipelines");
  return response.data.pipelines;
};

export const listDeals = async (token: string, page?: PageRequest) => {
  const api = createApiClient(token);
  const response = await api.get<DealPage>("/deals", {
    params: {
      ...(page?.limit !== undefined ? { limit: page.limit } : {}),
      ...(page?.offset !== undefined ? { offset: page.offset } : {}),
    },
  });
  return response.data;
};

export type CreateDealPayload = APISchemas["CreateDealRequest"];

export const createDeal = async (token: string, payload: CreateDealPayload) => {
  const api = createApiClient(token);
  const response = await api.post<{ deal: DealRecord }>("/deals", payload);
  return response.data.deal;
};

export const fetchDeal = async (token: string, dealId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ deal: DealRecord }>(`/deals/${dealId}`);
  return response.data.deal;
};

export const updateDeal = async (
  token: string,
  dealId: number,
  payload: Partial<CreateDealPayload> & { status?: string },
) => {
  const api = createApiClient(token);
  const response = await api.patch<{ deal: DealRecord }>(
    `/deals/${dealId}`,
    payload,
  );
  return response.data.deal;
};

export const addDealContact = async (
  token: string,
  dealId: number,
  contactId: number,
) => {
  const api = createApiClient(token);
  await api.post(`/deals/${dealId}/contacts`, { contact_id: contactId });
};

export const listDealActivities = async (token: string, dealId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ activities: DealActivityRecord[] }>(
    `/deals/${dealId}/activities`,
  );
  return response.data.activities;
};
