import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { createOrganization, listOrganizations, switchOrganization, type Organization } from "@/api/identity";
import { setOrganizationId } from "@/api/http";
import { useAuth } from "@/auth/AuthContext";
import { OrganizationContext } from "@/organizations/OrganizationContext";

const storageKey = "allcallall.activeOrganizationId";

export function OrganizationProvider({ children }: { children: React.ReactNode }) {
  const { status } = useAuth();
  const queryClient = useQueryClient();
  const [activeId, setActiveId] = useState<number | null>(() => Number(localStorage.getItem(storageKey)) || null);
  const query = useQuery({ queryKey: ["organizations"], queryFn: listOrganizations, enabled: status === "authenticated" });
  const organizations = useMemo(() => query.data ?? [], [query.data]);
  const activeOrganization = organizations.find((item) => item.id === activeId) ?? organizations[0] ?? null;

  useEffect(() => {
    const id = activeOrganization?.id ?? null;
    setOrganizationId(id);
    if (id) localStorage.setItem(storageKey, String(id));
  }, [activeOrganization?.id]);

  const select = useCallback(async (id: number) => {
    if (id === activeOrganization?.id) return;
    await switchOrganization(id);
    setActiveId(id);
    setOrganizationId(id);
    await queryClient.cancelQueries();
    queryClient.removeQueries({ predicate: (item) => item.queryKey[0] !== "organizations" });
  }, [activeOrganization?.id, queryClient]);

  const create = useCallback(async (name: string) => {
    const organization = await createOrganization(name);
    queryClient.setQueryData<Organization[]>(["organizations"], (items = []) => [...items, organization]);
    await select(organization.id);
    return organization;
  }, [queryClient, select]);

  const value = useMemo(() => ({ organizations, activeOrganization, loading: query.isLoading, select, create }), [organizations, activeOrganization, query.isLoading, select, create]);
  return <OrganizationContext.Provider value={value}>{children}</OrganizationContext.Provider>;
}
