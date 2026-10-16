import { beforeEach, describe, expect, it, vi } from "vitest";

const apiRequestMock = vi.hoisted(() => vi.fn());
vi.mock("@/api/http", () => ({ apiRequest: apiRequestMock }));

import * as realtime from "@/api/realtime";

beforeEach(() => {
  apiRequestMock.mockReset();
});

describe("realtime api", () => {
  it("issueRealtimeTicket posts the requested channel", async () => {
    apiRequestMock.mockResolvedValue({ token: "t", channel: "chat" });
    await realtime.issueRealtimeTicket("chat");
    expect(apiRequestMock).toHaveBeenCalledWith("/realtime/tickets", {
      method: "POST",
      body: JSON.stringify({ channel: "chat" }),
    });
  });

  it("getWebRTCConfig requests the ICE servers", async () => {
    apiRequestMock.mockResolvedValue({ ice_servers: [] });
    await realtime.getWebRTCConfig();
    expect(apiRequestMock).toHaveBeenCalledWith("/webrtc/config");
  });
});
