import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

// This runtime does not expose localStorage (Node's experimental global shadows
// the jsdom one and resolves to undefined), but @/i18n and OrganizationProvider
// read it at module/initialization scope. Install a tiny in-memory stand-in
// before those imports; vi.hoisted is hoisted above all imports by vitest.
vi.hoisted(() => {
  const store = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      clear: () => store.clear(),
      getItem: (key: string) => store.get(key) ?? null,
      key: (index: number) => [...store.keys()][index] ?? null,
      get length() {
        return store.size;
      },
      removeItem: (key: string) => {
        store.delete(key);
      },
      setItem: (key: string, value: string) => {
        store.set(key, String(value));
      },
    },
  });
});

const {
  listOrganizationsMock,
  createOrganizationMock,
  switchOrganizationMock,
  listOrganizationMembersMock,
  listOrganizationInvitesMock,
  listOrganizationTeamsMock,
  getOrganizationPolicyMock,
  listOrganizationAuditEventsMock,
  getOrganizationAdminSummaryMock,
} = vi.hoisted(() => ({
  listOrganizationsMock: vi.fn<() => Promise<unknown>>(),
  createOrganizationMock: vi.fn<(name: string) => Promise<unknown>>(),
  switchOrganizationMock: vi.fn<(id: number) => Promise<unknown>>(),
  listOrganizationMembersMock: vi.fn<(id: number, page?: unknown) => Promise<unknown>>(),
  listOrganizationInvitesMock: vi.fn<(id: number) => Promise<unknown>>(),
  listOrganizationTeamsMock: vi.fn<(id: number) => Promise<unknown>>(),
  getOrganizationPolicyMock: vi.fn<(id: number) => Promise<unknown>>(),
  listOrganizationAuditEventsMock: vi.fn<(id: number) => Promise<unknown>>(),
  getOrganizationAdminSummaryMock: vi.fn<(id: number) => Promise<unknown>>(),
}));

vi.mock("@/api/identity", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/identity")>()),
  listOrganizations: listOrganizationsMock,
  createOrganization: createOrganizationMock,
  switchOrganization: switchOrganizationMock,
  listOrganizationMembers: listOrganizationMembersMock,
  listOrganizationInvites: listOrganizationInvitesMock,
  listOrganizationTeams: listOrganizationTeamsMock,
  getOrganizationPolicy: getOrganizationPolicyMock,
  listOrganizationAuditEvents: listOrganizationAuditEventsMock,
  getOrganizationAdminSummary: getOrganizationAdminSummaryMock,
}));
vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({ status: "authenticated", user: { id: 1, name: "Ada" } }),
}));
vi.mock("@/pages/organizations/OrganizationAdminTabs", () => ({
  Overview: () => null,
  MembersTab: () => null,
  InvitesTab: () => null,
  TeamsTab: () => null,
  PoliciesTab: () => null,
  AuditTab: () => null,
}));

import i18n from "@/i18n";
import { OrganizationProvider } from "@/organizations/OrganizationProvider";
import { OrganizationsPage } from "@/pages/OrganizationsPage";

beforeAll(async () => {
  await i18n.changeLanguage("zh");
});

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <OrganizationProvider>
        <OrganizationsPage />
      </OrganizationProvider>
    </QueryClientProvider>,
  );
}

describe("OrganizationsPage states", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    listOrganizationsMock.mockResolvedValue([]);
    createOrganizationMock.mockResolvedValue({ id: 9, name: "新组织", slug: "new", role: "owner" });
    switchOrganizationMock.mockResolvedValue({ id: 1, name: "Alpha", slug: "alpha", role: "owner" });
    listOrganizationMembersMock.mockResolvedValue({
      members: [],
      pagination: { total: 0, limit: 50, offset: 0, has_more: false },
    });
    listOrganizationInvitesMock.mockResolvedValue([]);
    listOrganizationTeamsMock.mockResolvedValue([]);
    getOrganizationPolicyMock.mockResolvedValue({});
    listOrganizationAuditEventsMock.mockResolvedValue([]);
    getOrganizationAdminSummaryMock.mockResolvedValue({});
  });
  afterEach(cleanup);

  it("shows an empty state when the member has no organizations", async () => {
    renderPage();
    expect(await screen.findByText("还没有组织")).toBeInTheDocument();
  });

  it("surfaces the organization list error with a retry action", async () => {
    listOrganizationsMock.mockRejectedValue(new Error("组织列表加载失败"));
    renderPage();
    expect(await screen.findByText("组织列表加载失败")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();
  });

  it("blocks duplicate create submissions while creation is in flight", async () => {
    createOrganizationMock.mockImplementation(() => new Promise(() => {}));
    renderPage();
    const input = await screen.findByLabelText("组织名称");
    fireEvent.change(input, { target: { value: "新组织" } });
    const button = screen.getByRole("button", { name: "创建并切换" });
    fireEvent.click(button);
    await waitFor(() => expect(createOrganizationMock).toHaveBeenCalledTimes(1));
    fireEvent.click(button);
    expect(createOrganizationMock).toHaveBeenCalledTimes(1);
  });

  it("surfaces organization switch failures next to the list", async () => {
    listOrganizationsMock.mockResolvedValue([
      { id: 1, name: "Alpha", slug: "alpha", role: "owner" },
      { id: 2, name: "Beta", slug: "beta", role: "member" },
    ]);
    switchOrganizationMock.mockRejectedValue(new Error("切换服务不可用"));
    renderPage();
    const beta = await screen.findByRole("button", { name: /Beta/ });
    fireEvent.click(beta);
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("切换服务不可用");
  });
});
