import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { AppState, type AppStateStatus } from "react-native";

import {
  fetchFollowUps,
  type FollowUpListItem,
  updateFollowUpTask
} from "../api/commercial";
import { useAuthContext } from "./AuthContext";
import { useOrganization } from "./OrganizationContext";

interface FollowUpContextValue {
  items: FollowUpListItem[];
  loading: boolean;
  /**
   * Set when the refresh failed. Previously this was swallowed into
   * console.warn, so a network failure rendered as "暂无待跟进任务" - the user
   * saw an empty list and concluded there was nothing to do.
   */
  error: Error | null;
  refreshFollowUps: () => Promise<void>;
  completeTask: (taskId: number) => Promise<void>;
}

const FollowUpContext = createContext<FollowUpContextValue | undefined>(undefined);

export const FollowUpProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { token } = useAuthContext();
  const [items, setItems] = useState<FollowUpListItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const refreshFollowUps = useCallback(async () => {
    if (!token) {
      setItems([]);
      setError(null);
      setLoading(false);
      return;
    }
    try {
      setLoading(true);
      const next = await fetchFollowUps(token);
      setItems(next);
      setError(null);
    } catch (caught) {
      console.warn("[FollowUpContext] Failed to refresh follow-ups:", caught);
      // Keep whatever was already loaded: a failed refresh should not look
      // like the tasks disappeared.
      setError(caught instanceof Error ? caught : new Error(String(caught)));
    } finally {
      setLoading(false);
    }
  }, [token]);

  // Reload when the active organization changes, so the previous workspace's
  // follow-ups do not linger. See OrganizationContext.organizationVersion.
  const { organizationVersion } = useOrganization();

  useEffect(() => {
    void refreshFollowUps();
  }, [refreshFollowUps, organizationVersion]);

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
    error,
    refreshFollowUps,
    completeTask
  }), [items, loading, error, refreshFollowUps, completeTask]);

  return <FollowUpContext.Provider value={value}>{children}</FollowUpContext.Provider>;
};

export const useFollowUps = () => {
  const ctx = useContext(FollowUpContext);
  if (!ctx) {
    throw new Error("useFollowUps must be used within FollowUpProvider");
  }
  return ctx;
};
