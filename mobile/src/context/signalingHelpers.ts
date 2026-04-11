import type { RTCIceServerConfig } from "./signalingTypes";

export const preferRestrictedIceServers = (
  servers: RTCIceServerConfig[],
  restrictedNetworkMode: boolean
) => {
  if (!restrictedNetworkMode) return servers;

  const urlsOf = (server: RTCIceServerConfig) =>
    Array.isArray(server.urls) ? server.urls : [server.urls];

  const scoreUrl = (url: string) => {
    const lower = url.toLowerCase();
    if (lower.startsWith("turns:")) return lower.includes("transport=tcp") ? 0 : 1;
    if (lower.startsWith("turn:")) return lower.includes("transport=tcp") ? 2 : 3;
    if (lower.startsWith("stun:")) return 4;
    return 5;
  };

  const scoreServer = (server: RTCIceServerConfig) =>
    Math.min(...urlsOf(server).map(scoreUrl));

  return [...servers].sort((left, right) => scoreServer(left) - scoreServer(right));
};
