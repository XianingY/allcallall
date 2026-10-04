export const conversationKeys = {
  all: (organizationId?: number) => ["organizations", organizationId, "conversations"] as const,
  list: (organizationId: number | undefined, status: string) =>
    [...conversationKeys.all(organizationId), status] as const,
  detail: (organizationId: number | undefined, conversationId: number | null) =>
    [...conversationKeys.all(organizationId), conversationId] as const,
  messages: (organizationId: number | undefined, conversationId: number | null) =>
    [...conversationKeys.detail(organizationId, conversationId), "messages"] as const,
  pins: (organizationId: number | undefined, conversationId: number | null) =>
    [...conversationKeys.detail(organizationId, conversationId), "pins"] as const,
  notes: (organizationId: number | undefined, conversationId: number | null) =>
    [...conversationKeys.detail(organizationId, conversationId), "notes"] as const,
  messageSearch: (organizationId: number | undefined, query: string) =>
    ["organizations", organizationId, "search", "messages", query] as const,
};
