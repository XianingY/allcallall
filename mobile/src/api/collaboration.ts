import { createApiClient, getActiveOrganizationHeader } from "./client";
import { API_BASE_URL } from "../config";

export interface OrganizationRecord {
  id: number;
  name: string;
  slug: string;
  description?: string;
  role: string;
}

export interface OrganizationPolicyRecord {
  id: number;
  organization_id: number;
  recording_mode: string;
  recording_storage_days: number;
  recording_export_allowed: boolean;
}

export interface ConversationRecord {
  id: number;
  organization_id: number;
  team_id?: number | null;
  room_id?: number | null;
  type: string;
  title: string;
  topic?: string;
  status: string;
  assignee_user_id?: number | null;
  assignee_email?: string;
  assignee_display_name?: string;
  priority: string;
  contact_id?: number | null;
  last_internal_note_at?: string | null;
  last_message_at?: string | null;
  last_message_preview?: string;
  last_message_type?: string;
  unread_count: number;
  active_room_id?: number | null;
  active_room_title?: string;
  latest_room_id?: number | null;
  latest_room_title?: string;
  latest_recording_id?: number | null;
}

export interface ConversationNoteRecord {
  id: number;
  organization_id: number;
  conversation_id: number;
  author_id: number;
  author_email: string;
  author_display_name: string;
  body: string;
  created_at: string;
}

export interface ConversationFollowupRecord {
  call_id?: string;
  summary_cn?: string;
  summary_en?: string;
  action_items?: string[];
  next_step?: string;
}

export interface MeetingSummaryCard {
  summary: string;
  action_items?: string[];
  next_step?: string;
  assignee?: string;
}

export interface ConversationWorkspaceRecord {
  latest_meeting?: RoomListItemRecord | null;
  latest_recording?: RecordingRecord | null;
  meeting_summary?: MeetingSummaryCard | null;
  latest_note?: ConversationNoteRecord | null;
  agent_context: {
    latest_call_id?: string;
    transcript_segment_count: number;
    latest_transcript_at?: string | null;
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

export interface ConversationDetailRecord {
  conversation: ConversationRecord;
  latest_note?: ConversationNoteRecord | null;
  latest_room?: RoomListItemRecord | null;
  latest_followup?: ConversationFollowupRecord | null;
  workspace: ConversationWorkspaceRecord;
}

export interface MessageRecord {
  id: number;
  organization_id: number;
  conversation_id: number;
  sender_id: number;
  sender_email: string;
  sender_display_name: string;
  type: string;
  body: string;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface RoomMemberRecord {
  id: number;
  room_id: number;
  user_id: number;
  role: string;
  user_email?: string;
  user_display_name?: string;
  joined?: boolean;
  left?: boolean;
  audio_enabled?: boolean;
  video_enabled?: boolean;
  connection_state?: string;
  is_host?: boolean;
  joined_at?: string | null;
  left_at?: string | null;
}

export interface RoomEventRecord {
  id: number;
  room_id: number;
  user_id: number;
  type: string;
  payload_json?: string;
  created_at: string;
}

export interface RecordingSessionRecord {
  id: number;
  organization_id: number;
  room_id: number;
  started_by: number;
  status: string;
  started_at?: string | null;
  stopped_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface RecordingFileRecord {
  id: number;
  recording_session_id: number;
  storage_driver: string;
  storage_bucket?: string;
  object_key: string;
  etag?: string;
  content_type: string;
  retention_until?: string | null;
  deleted_at?: string | null;
  duration_seconds: number;
  metadata_json?: string;
  created_at: string;
  download_url: string;
  file_name: string;
  file_size_bytes: number;
  recording_kind: string;
}

export interface RecordingRecord {
  session: RecordingSessionRecord;
  files: RecordingFileRecord[];
}

export interface RoomRecord {
  room: {
    id: number;
    organization_id: number;
    team_id?: number | null;
    conversation_id?: number | null;
    title: string;
    status: string;
    created_by: number;
    started_at?: string | null;
    ended_at?: string | null;
    created_at: string;
    updated_at: string;
  };
  members: RoomMemberRecord[];
  events: RoomEventRecord[];
  active_recording?: RecordingSessionRecord | null;
  conversation_id?: number | null;
  conversation_title?: string;
  participant_count: number;
  is_active: boolean;
  has_recording: boolean;
  latest_recording_id?: number | null;
}

export interface RoomListItemRecord {
  id: number;
  organization_id: number;
  team_id?: number | null;
  conversation_id?: number | null;
  conversation_title?: string;
  title: string;
  status: string;
  created_by: number;
  started_at?: string | null;
  ended_at?: string | null;
  created_at: string;
  updated_at: string;
  participant_count: number;
  is_active: boolean;
  has_recording: boolean;
  latest_recording_id?: number | null;
}

export interface RoomOfferAnswer {
  type: string;
  sdp: string;
}

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

export interface PipelineStageRecord {
  id: number;
  pipeline_id: number;
  name: string;
  position: number;
  is_closed: boolean;
}

export interface PipelineRecord {
  id: number;
  organization_id: number;
  name: string;
  is_default: boolean;
  stages: PipelineStageRecord[];
}

export interface DealRecord {
  id: number;
  organization_id: number;
  pipeline_id: number;
  stage_id?: number | null;
  stage_name?: string;
  owner_id: number;
  title: string;
  description?: string;
  status: string;
  value_cents: number;
  currency: string;
  created_at: string;
  updated_at: string;
}

export interface DealActivityRecord {
  id: number;
  organization_id: number;
  deal_id: number;
  type: string;
  reference_type: string;
  reference_id: string;
  summary: string;
  metadata_json?: string;
  created_by: number;
  created_at: string;
}

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

export const listConversations = async (
  token: string,
  filter?: string,
  contactId?: number,
) => {
  const api = createApiClient(token);
  const response = await api.get<{ conversations: ConversationRecord[] }>(
    "/conversations",
    {
      params: {
        ...(filter ? { filter } : {}),
        ...(contactId ? { contact_id: contactId } : {}),
      },
    },
  );
  return response.data.conversations;
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

export interface CreateConversationPayload {
  type: string;
  title?: string;
  topic?: string;
  member_ids?: number[];
  team_id?: number;
}

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

export interface UpdateConversationPayload {
  status?: string;
  assignee_user_id?: number | null;
  priority?: string;
  contact_id?: number | null;
}

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

export interface CreateRoomPayload {
  title: string;
  participant_ids?: number[];
  team_id?: number;
  conversation_id?: number;
}

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

export interface RoomIceCandidatePayload {
  candidate?: string;
  sdpMid?: string | null;
  sdpMLineIndex?: number | null;
}

export interface RoomMediaStatePayload {
  audio_enabled?: boolean;
  video_enabled?: boolean;
  connection_state?: string;
}

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

export const listRecordings = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ recordings: RecordingRecord[] }>(
    "/recordings",
  );
  return response.data.recordings;
};

export const fetchRecording = async (token: string, recordingId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ recording: RecordingRecord }>(
    `/recordings/${recordingId}`,
  );
  return response.data.recording;
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

export const listMessages = async (token: string, conversationId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ messages: MessageRecord[] }>(
    `/conversations/${conversationId}/messages`,
  );
  return response.data.messages;
};

export interface CreateMessagePayload {
  type?: string;
  body: string;
  metadata?: Record<string, unknown>;
}

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

export const listDeals = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ deals: DealRecord[] }>("/deals");
  return response.data.deals;
};

export interface CreateDealPayload {
  title: string;
  description?: string;
  value_cents?: number;
  currency?: string;
  stage_id?: number;
}

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
