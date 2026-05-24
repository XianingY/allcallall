import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";

import {
  createOrganization,
  listOrganizations,
  OrganizationRecord,
  switchOrganization
} from "../api/collaboration";
import { setActiveOrganizationHeader } from "../api/client";
import { useAuthContext } from "./AuthContext";

const ACTIVE_ORG_STORAGE_KEY = "@allcallall:active-organization";

interface OrganizationContextValue {
  organizations: OrganizationRecord[];
  currentOrganization: OrganizationRecord | null;
  loading: boolean;
  refreshOrganizations: () => Promise<void>;
  selectOrganization: (organizationId: number) => Promise<void>;
  createWorkspace: (name: string) => Promise<OrganizationRecord>;
}

const OrganizationContext = createContext<OrganizationContextValue | undefined>(undefined);

export const OrganizationProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { token } = useAuthContext();
  const [organizations, setOrganizations] = useState<OrganizationRecord[]>([]);
  const [currentOrganization, setCurrentOrganization] = useState<OrganizationRecord | null>(null);
  const [loading, setLoading] = useState(true);

  const applyOrganization = useCallback(async (items: OrganizationRecord[], preferredId?: number | null) => {
    if (items.length === 0) {
      setOrganizations([]);
      setCurrentOrganization(null);
      setActiveOrganizationHeader(null);
      return;
    }
    const fallback = items[0];
    const target = items.find((item) => item.id === preferredId) ?? fallback;
    setOrganizations(items);
    setCurrentOrganization(target);
    setActiveOrganizationHeader(target.id);
    await AsyncStorage.setItem(ACTIVE_ORG_STORAGE_KEY, String(target.id));
  }, []);

  const refreshOrganizations = useCallback(async () => {
    if (!token) {
      setOrganizations([]);
      setCurrentOrganization(null);
      setActiveOrganizationHeader(null);
      setLoading(false);
      return;
    }
    try {
      setLoading(true);
      const [items, storedId] = await Promise.all([
        listOrganizations(token),
        AsyncStorage.getItem(ACTIVE_ORG_STORAGE_KEY)
      ]);
      const preferredId = storedId ? Number(storedId) : null;
      await applyOrganization(items, preferredId);
    } catch (error) {
      console.warn("[OrganizationContext] Failed to load organizations:", error);
      setOrganizations([]);
      setCurrentOrganization(null);
      setActiveOrganizationHeader(null);
    } finally {
      setLoading(false);
    }
  }, [applyOrganization, token]);

  useEffect(() => {
    void refreshOrganizations();
  }, [refreshOrganizations]);

  const selectOrganization = useCallback(async (organizationId: number) => {
    if (!token) {
      return;
    }
    const org = await switchOrganization(token, organizationId);
    setCurrentOrganization(org);
    setActiveOrganizationHeader(org.id);
    await AsyncStorage.setItem(ACTIVE_ORG_STORAGE_KEY, String(org.id));
  }, [token]);

  const createWorkspace = useCallback(async (name: string) => {
    if (!token) {
      throw new Error("missing auth token");
    }
    const organization = await createOrganization(token, name);
    await refreshOrganizations();
    await selectOrganization(organization.id);
    return organization;
  }, [refreshOrganizations, selectOrganization, token]);

  const value = useMemo<OrganizationContextValue>(() => ({
    organizations,
    currentOrganization,
    loading,
    refreshOrganizations,
    selectOrganization,
    createWorkspace
  }), [organizations, currentOrganization, loading, refreshOrganizations, selectOrganization, createWorkspace]);

  return <OrganizationContext.Provider value={value}>{children}</OrganizationContext.Provider>;
};

export const useOrganization = () => {
  const ctx = useContext(OrganizationContext);
  if (!ctx) {
    throw new Error("useOrganization must be used within OrganizationProvider");
  }
  return ctx;
};
