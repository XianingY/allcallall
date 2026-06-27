import { create } from "zustand";

export type CallStatus = "idle" | "outgoing" | "incoming" | "connecting" | "connected" | "reconnecting" | "failed";

interface CallState {
  status: CallStatus;
  callId: string;
  peerEmail: string;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  muted: boolean;
  cameraEnabled: boolean;
  error: string;
  patch(value: Partial<Omit<CallState, "patch" | "reset">>): void;
  reset(): void;
}

const initial = { status: "idle" as const, callId: "", peerEmail: "", localStream: null, remoteStream: null, muted: false, cameraEnabled: false, error: "" };

export const useCallStore = create<CallState>((set) => ({
  ...initial,
  patch: (value) => set(value),
  reset: () => set(initial),
}));

