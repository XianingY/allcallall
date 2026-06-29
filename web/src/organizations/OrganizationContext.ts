import { createContext, useContext } from "react";

import type { Organization } from "@/api/identity";

export interface OrganizationContextValue {
  organizations: Organization[];
  activeOrganization: Organization | null;
  loading: boolean;
  select(id: number): Promise<void>;
  create(name: string): Promise<Organization>;
}

export const OrganizationContext = createContext<OrganizationContextValue | null>(null);

export function useOrganization() {
  const value = useContext(OrganizationContext);
  if (!value) throw new Error("useOrganization must be used inside OrganizationProvider");
  return value;
}
