import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState
} from "react";
import { AppState, type AppStateStatus } from "react-native";

import {
  acceptLegal,
  fetchEntitlements,
  fetchUsage,
  type EntitlementRecord,
  type UsageRecord
} from "../api/commercial";
import { useAuthContext } from "./AuthContext";
import { useOrganization } from "./OrganizationContext";
import BillingService from "../services/BillingService";

interface CommercialState {
  tier: "free" | "premium";
  entitlements: EntitlementRecord[];
  usage: UsageRecord[];
  loading: boolean;
  /**
   * Set when entitlements/usage could not be fetched. Kept separate from
   * `tier` so a failed request is not reported as "free" - which is what a
   * paying user would see if the request failed.
   */
  error: Error | null;
}

interface CommercialContextValue extends CommercialState {
  refreshCommercialState: () => Promise<void>;
  markLegalAccepted: () => Promise<void>;
}

const CommercialContext = createContext<CommercialContextValue | undefined>(undefined);

export const CommercialProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { token } = useAuthContext();
  const [state, setState] = useState<CommercialState>({
    tier: "free",
    entitlements: [],
    usage: [],
    loading: false,
    error: null
  });

  const refreshCommercialState = useCallback(async () => {
    if (!token) {
      setState({
        tier: "free",
        entitlements: [],
        usage: [],
        loading: false,
        error: null
      });
      return;
    }

    setState((current) => ({ ...current, loading: true }));
    try {
      const [entitlementResponse, usage] = await Promise.all([
        fetchEntitlements(token),
        fetchUsage(token)
      ]);
      setState({
        tier: entitlementResponse.tier === "premium" ? "premium" : "free",
        entitlements: entitlementResponse.entitlements,
        usage,
        loading: false,
        error: null
      });
    } catch (error) {
      // Surface it: swallowing this meant a failed entitlement/usage fetch was
      // indistinguishable from a free account, including for paying users.
      console.warn("[CommercialContext] Failed to refresh commercial state:", error);
      setState((current) => ({
        ...current,
        loading: false,
        error: error instanceof Error ? error : new Error(String(error))
      }));
    }
  }, [token]);

  // Entitlements and usage are per-organization, so a workspace switch has to
  // drop the previous values rather than keep showing them. See
  // OrganizationContext.organizationVersion.
  const { organizationVersion } = useOrganization();

  useEffect(() => {
    void refreshCommercialState();
  }, [refreshCommercialState, organizationVersion]);

  useEffect(() => {
    if (!token) {
      return;
    }

    const subscription = AppState.addEventListener("change", (nextState: AppStateStatus) => {
      if (nextState !== "active") {
        return;
      }
      void (async () => {
        try {
          await BillingService.getCustomerInfo();
        } catch (error) {
          console.warn("[CommercialContext] Failed to refresh RevenueCat customer info:", error);
        }
        await refreshCommercialState();
      })();
    });

    return () => {
      subscription.remove();
    };
  }, [refreshCommercialState, token]);

  const markLegalAccepted = useCallback(async () => {
    if (!token) {
      return;
    }
    await acceptLegal(token);
  }, [token]);

  const value = useMemo<CommercialContextValue>(
    () => ({
      ...state,
      refreshCommercialState,
      markLegalAccepted
    }),
    [state, refreshCommercialState, markLegalAccepted]
  );

  return <CommercialContext.Provider value={value}>{children}</CommercialContext.Provider>;
};

export const useCommercial = () => {
  const ctx = useContext(CommercialContext);
  if (!ctx) {
    throw new Error("useCommercial must be used within CommercialProvider");
  }
  return ctx;
};
