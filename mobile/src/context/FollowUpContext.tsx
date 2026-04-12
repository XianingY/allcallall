import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { AppState, type AppStateStatus } from "react-native";

import {
  fetchFollowUps,
  type FollowUpListItem,
  updateFollowUpTask
} from "../api/commercial";
import { useAuthContext } from "./AuthContext";

interface FollowUpContextValue {
  items: FollowUpListItem[];
  loading: boolean;
  refreshFollowUps: () => Promise<void>;
  completeTask: (taskId: number) => Promise<void>;
}

const FollowUpContext = createContext<FollowUpContextValue | undefined>(undefined);

export const FollowUpProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { token } = useAuthContext();
  const [items, setItems] = useState<FollowUpListItem[]>([]);
  const [loading, setLoading] = useState(false);

  const refreshFollowUps = useCallback(async () => {
    if (!token) {
      setItems([]);
      setLoading(false);
      return;
    }
    try {
      setLoading(true);
      const next = await fetchFollowUps(token);
      setItems(next);
    } catch (error) {
      console.warn("[FollowUpContext] Failed to refresh follow-ups:", error);
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    void refreshFollowUps();
  }, [refreshFollowUps]);

  useEffect(() => {
    if (!token) {
      return;
    }
    const subscription = AppState.addEventListener("change", (nextState: AppStateStatus) => {
      if (nextState === "active") {
        void refreshFollowUps();
      }
    });
    return () => subscription.remove();
  }, [refreshFollowUps, token]);

  const completeTask = useCallback(async (taskId: number) => {
    if (!token) {
      return;
    }
    await updateFollowUpTask(token, taskId, { status: "done" });
    await refreshFollowUps();
  }, [refreshFollowUps, token]);

  const value = useMemo<FollowUpContextValue>(() => ({
    items,
    loading,
    refreshFollowUps,
    completeTask
  }), [items, loading, refreshFollowUps, completeTask]);

  return <FollowUpContext.Provider value={value}>{children}</FollowUpContext.Provider>;
};

export const useFollowUps = () => {
  const ctx = useContext(FollowUpContext);
  if (!ctx) {
    throw new Error("useFollowUps must be used within FollowUpProvider");
  }
  return ctx;
};
