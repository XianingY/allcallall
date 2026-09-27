import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

const {
  createFileSourceMock,
  createTextSourceMock,
  createURLSourceMock,
  decideDuplicateMock,
  getKnowledgeSourceMock,
  listDeadLettersMock,
  listDuplicatesMock,
  listKnowledgeSourcesMock,
  listSourceGroupsMock,
  reingestSourceMock,
  retryDeadLetterMock,
  setCanonicalSourceMock,
} = vi.hoisted(() => ({
  createFileSourceMock: vi.fn(),
  createTextSourceMock: vi.fn(),
  createURLSourceMock: vi.fn(),
  decideDuplicateMock: vi.fn(),
  getKnowledgeSourceMock: vi.fn(),
  listDeadLettersMock: vi.fn(),
  listDuplicatesMock: vi.fn(),
  listKnowledgeSourcesMock: vi.fn(),
  listSourceGroupsMock: vi.fn(),
  reingestSourceMock: vi.fn(),
  retryDeadLetterMock: vi.fn(),
  setCanonicalSourceMock: vi.fn(),
}));

vi.mock("@/api/knowledge", () => ({
  createFileSource: () => createFileSourceMock(),
  createTextSource: () => createTextSourceMock(),
  createURLSource: () => createURLSourceMock(),
  decideDuplicate: (id: number, decision: string) => decideDuplicateMock(id, decision),
  getKnowledgeSource: (id: number) => getKnowledgeSourceMock(id),
  listDeadLetters: () => listDeadLettersMock(),
  listDuplicates: () => listDuplicatesMock(),
  listKnowledgeSources: () => listKnowledgeSourcesMock(),
  listSourceGroups: () => listSourceGroupsMock(),
  reingestSource: (id: number) => reingestSourceMock(id),
  retryDeadLetter: (id: number) => retryDeadLetterMock(id),
  setCanonicalSource: (groupId: number, sourceId: number) => setCanonicalSourceMock(groupId, sourceId),
}));

vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({
    organizations: [],
    activeOrganization: { id: 7, name: "Demo", slug: "demo", role: "owner" },
    loading: false,
    select: () => Promise.resolve(),
    create: () => Promise.resolve({}),
  }),
}));

import { KnowledgePage } from "@/pages/knowledge/KnowledgePage";

const source = {
  id: 1,
  organization_id: 7,
  created_by: 1,
  kind: "manual_text",
  title: "产品手册",
  authority_score: 0.9,
  dedupe_status: "unique",
  status: "ready",
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

const renderPage = (entries: string[] = ["/knowledge"]) =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter initialEntries={entries}>
        <KnowledgePage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const mockEmptyLists = () => {
  listKnowledgeSourcesMock.mockResolvedValue([]);
  listDuplicatesMock.mockResolvedValue([]);
  listSourceGroupsMock.mockResolvedValue([]);
  listDeadLettersMock.mockResolvedValue([]);
};

describe("KnowledgePage", () => {
  afterEach(cleanup);

  it("shows an empty state when the user has no knowledge sources", async () => {
    mockEmptyLists();
    renderPage();

    await waitFor(() => expect(screen.getByText("还没有知识来源")).toBeInTheDocument());
  });

  it("shows an error when reingesting a source fails", async () => {
    mockEmptyLists();
    listKnowledgeSourcesMock.mockResolvedValue([source]);
    getKnowledgeSourceMock.mockResolvedValue({ source, versions: [], chunks: [] });
    reingestSourceMock.mockRejectedValue(new Error("重新索引失败"));
    renderPage(["/knowledge?sourceId=1"]);

    fireEvent.click(await screen.findByRole("button", { name: /重新索引/ }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("重新索引失败"));
  });
});
