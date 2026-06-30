import type { OrganizationInvite, OrganizationMember, OrganizationTeam } from "@/api/identity";

export function filterMembers(items: OrganizationMember[], search: string, role: string) {
  const keyword = normalize(search);
  return items.filter((item) => {
    const matchRole = !role || item.role === role;
    const haystack = normalize(`${item.display_name} ${item.email} ${item.status}`);
    return matchRole && (!keyword || haystack.includes(keyword));
  });
}

export function filterInvites(items: OrganizationInvite[], search: string, status: string) {
  const keyword = normalize(search);
  return items.filter((item) => {
    const matchStatus = !status || item.status === status;
    const haystack = normalize(`${item.target_email} ${item.role} ${item.status} ${item.code}`);
    return matchStatus && (!keyword || haystack.includes(keyword));
  });
}

export function filterTeams(items: OrganizationTeam[], search: string) {
  const keyword = normalize(search);
  if (!keyword) return items;
  return items.filter((item) => normalize(`${item.name} ${item.slug} ${item.description}`).includes(keyword));
}

function normalize(value?: string | null) {
  return (value ?? "").trim().toLowerCase();
}
