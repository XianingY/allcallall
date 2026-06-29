import { createContext, useContext } from "react";

export interface CallContextValue {
  start(email: string): Promise<void>;
  accept(): Promise<void>;
  reject(): void;
  end(): void;
  toggleMute(): void;
  toggleCamera(): Promise<void>;
  switchInput(deviceId: string): Promise<void>;
}

export const CallContext = createContext<CallContextValue | null>(null);

export function useCall() {
  const value = useContext(CallContext);
  if (!value) throw new Error("useCall must be used inside CallProvider");
  return value;
}
