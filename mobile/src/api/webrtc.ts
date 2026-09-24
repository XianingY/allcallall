import type { operations } from "@allcallall/api-types";

import { createApiClient } from "./client";

// #22：改用 openapi.yaml 生成的共享契约，删除手写响应类型。
// 保留 WebRTCConfigResponse 这一导出名，使下游（SignalingContext / RoomCallContext）
// 无需任何改动——这是"分阶段迁移"能保持零回归的关键。
export type WebRTCConfigResponse =
  operations["getWebRTCConfig"]["responses"][200]["content"]["application/json"];

export const fetchWebRTCConfig = async (
  token: string
): Promise<WebRTCConfigResponse> => {
  const api = createApiClient(token);
  const { data } = await api.get<WebRTCConfigResponse>("/webrtc/config");
  return data;
};
