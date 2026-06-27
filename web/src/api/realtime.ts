import { apiRequest } from "@/api/http";
import type { components } from "@/api/schema";

export type RealtimeTicket = components["schemas"]["RealtimeTicket"];

export const issueRealtimeTicket = (channel: RealtimeTicket["channel"]) => apiRequest<RealtimeTicket>("/realtime/tickets", { method: "POST", body: JSON.stringify({ channel }) });
export const getWebRTCConfig = () => apiRequest<{ ice_servers: RTCIceServer[] }>("/webrtc/config");
