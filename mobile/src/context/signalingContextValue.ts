import { createContext, useContext } from "react";

import type { SignalingContextValue } from "./signalingTypes";

export const SignalingContext = createContext<SignalingContextValue | undefined>(
  undefined,
);

export const useSignaling = () => {
  const ctx = useContext(SignalingContext);
  if (!ctx) throw new Error("useSignaling must be used within SignalingProvider");
  return ctx;
};
