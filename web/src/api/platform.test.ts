import { beforeEach, describe, expect, it, vi } from "vitest";

const apiRequestMock = vi.hoisted(() => vi.fn());
vi.mock("@/api/http", () => ({ apiRequest: apiRequestMock }));

import * as platform from "@/api/platform";

beforeEach(() => {
  apiRequestMock.mockReset();
});

describe("platform api", () => {
  it("listPushDevices unwraps the devices array", async () => {
    apiRequestMock.mockResolvedValue({ devices: [{ id: 1 }] });
    await expect(platform.listPushDevices()).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/push/devices");
  });

  it("listPushDevices tolerates a missing array", async () => {
    apiRequestMock.mockResolvedValue({});
    await expect(platform.listPushDevices()).resolves.toEqual([]);
  });

  it("registerPushDevice posts the device payload", async () => {
    apiRequestMock.mockResolvedValue({ device: { id: 9 } });
    await platform.registerPushDevice({ token: "tok", provider: "web-push", platform: "web" });
    expect(apiRequestMock).toHaveBeenCalledWith("/push/devices", {
      method: "POST",
      body: JSON.stringify({ token: "tok", provider: "web-push", platform: "web" }),
    });
  });

  it("deletePushDevice issues a DELETE", async () => {
    apiRequestMock.mockResolvedValue(undefined);
    await platform.deletePushDevice(9);
    expect(apiRequestMock).toHaveBeenCalledWith("/push/devices/9", { method: "DELETE" });
  });

  it("getEntitlements requests the user tier", async () => {
    apiRequestMock.mockResolvedValue({ tier: "free", entitlements: [] });
    await platform.getEntitlements();
    expect(apiRequestMock).toHaveBeenCalledWith("/entitlements/me");
  });

  it("getUsage unwraps the usage snapshots", async () => {
    apiRequestMock.mockResolvedValue({ usage: [{ kind: "messages" }] });
    await expect(platform.getUsage()).resolves.toEqual([{ kind: "messages" }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/usage/me");
  });
});
