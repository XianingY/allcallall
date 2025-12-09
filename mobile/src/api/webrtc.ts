import { createApiClient } from "./client";

type IceServer = {
  urls: string[] | string;
  username?: string;
  credential?: string;
};

export interface WebRTCConfigResponse {
  ice_servers: IceServer[];
}

export const fetchWebRTCConfig = async (
  token: string
): Promise<WebRTCConfigResponse> => {
  const api = createApiClient(token);
  const { data } = await api.get<WebRTCConfigResponse>("/webrtc/config");
  return data;
};
