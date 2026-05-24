import { SignalingClient } from "../api/signaling";
import { PollingSignalingClient } from "../api/signalingPoll";

import type { SignalingTransport } from "./signalingTypes";

export const createSignalingTransport = (
  token: string,
  mode: string
): SignalingTransport => {
  if (mode === "poll") {
    return new PollingSignalingClient(token);
  }
  return new SignalingClient(token);
};
