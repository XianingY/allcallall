import type { components } from "@/api/schema";
import { apiRequest } from "@/api/http";

export type PushDevice = components["schemas"]["PushDevice"];
export type RegisterPushDeviceRequest = components["schemas"]["RegisterPushDeviceRequest"];
export type UserEntitlement = components["schemas"]["UserEntitlement"];
export type UsageSnapshot = components["schemas"]["UsageSnapshot"];

export const listPushDevices = () => apiRequest<{ devices: PushDevice[] }>("/push/devices").then((value) => value.devices ?? []);
export const registerPushDevice = (input: RegisterPushDeviceRequest) => apiRequest<{ device: PushDevice }>("/push/devices", { method: "POST", body: JSON.stringify(input) }).then((value) => value.device);
export const deletePushDevice = (id: number) => apiRequest<void>(`/push/devices/${id}`, { method: "DELETE" });

export const getEntitlements = () => apiRequest<{ tier: "free" | "premium"; entitlements: UserEntitlement[] }>("/entitlements/me");
export const getUsage = () => apiRequest<{ usage: UsageSnapshot[] }>("/usage/me").then((value) => value.usage ?? []);
