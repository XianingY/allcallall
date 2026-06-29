import type { Tab } from "@/pages/organizations/OrganizationAdminTabs";

export const organizationTabs: Tab[] = ["overview", "members", "invites", "teams", "policies", "audit"];

export function tabLabel(tab: Tab) {
  return ({ overview: "概览", members: "成员", invites: "邀请", teams: "团队", policies: "策略", audit: "审计" } as Record<Tab, string>)[tab];
}
