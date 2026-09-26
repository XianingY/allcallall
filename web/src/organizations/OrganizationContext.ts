import { createContext, useContext } from "react";

import type { Organization } from "@/api/identity";

export interface OrganizationContextValue {
  organizations: Organization[];
  activeOrganization: Organization | null;
  loading: boolean;
  /**
   * Set when the organization list could not be loaded. Every page query is
   * gated on `Boolean(orgId)`, so a failed request used to leave the whole app
   * looking empty while still logged in - which reads as "no data" rather than
   * "the request failed". Consumers must surface this with a retry.
   */
  error: Error | null;
  retry(): void;
  select(id: number): Promise<void>;
  create(name: string): Promise<Organization>;
}

export const OrganizationContext = createContext<OrganizationContextValue | null>(null);

export function useOrganization() {
  const value = useContext(OrganizationContext);
  if (!value) throw new Error("useOrganization must be used inside OrganizationProvider");
  return value;
}
