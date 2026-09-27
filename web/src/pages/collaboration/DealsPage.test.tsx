import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

const { createDealMock, listDealsMock, listPipelinesMock, updateDealMock } = vi.hoisted(
  () => ({
    createDealMock: vi.fn(),
    listDealsMock: vi.fn(),
    listPipelinesMock: vi.fn(),
    updateDealMock: vi.fn(),
  }),
);

vi.mock("@/api/collaboration", () => ({
  createDeal: () => createDealMock(),
  listDeals: (page?: unknown) => listDealsMock(page),
  listPipelines: () => listPipelinesMock(),
  updateDeal: (id: number, input: unknown) => updateDealMock(id, input),
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

import { DealsPage } from "@/pages/collaboration/DealsPage";

const pipelines = [
  {
    id: 1,
    organization_id: 7,
    name: "默认流水线",
    is_default: true,
    stages: [
      { id: 1, pipeline_id: 1, name: "线索", position: 0, is_closed: false },
      { id: 2, pipeline_id: 1, name: "赢单", position: 1, is_closed: false },
    ],
  },
];

const deal = {
  id: 101,
  organization_id: 7,
  pipeline_id: 1,
  stage_id: 1,
  owner_id: 1,
  title: "续约合同",
  description: "年度续约",
  status: "open",
  value_cents: 1200000,
  currency: "CNY",
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <DealsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("DealsPage", () => {
  afterEach(cleanup);

  it("shows an error when the stage-move mutation rejects", async () => {
    listPipelinesMock.mockResolvedValue(pipelines);
    listDealsMock.mockResolvedValue({
      deals: [deal],
      pagination: { total: 1, limit: 50, offset: 0, has_more: false },
    });
    updateDealMock.mockRejectedValue(new Error("移动商机失败"));
    renderPage();

    const select = await screen.findByLabelText("移动 续约合同");
    fireEvent.change(select, { target: { value: "2" } });

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("移动商机失败"));
  });

  it("keeps the create button disabled while the create mutation is pending", async () => {
    listPipelinesMock.mockResolvedValue(pipelines);
    listDealsMock.mockResolvedValue({
      deals: [],
      pagination: { total: 0, limit: 50, offset: 0, has_more: false },
    });
    createDealMock.mockReturnValue(new Promise(() => {}));
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /新建商机/ }));
    fireEvent.change(screen.getByLabelText("标题"), { target: { value: "续约合同" } });
    fireEvent.click(screen.getByRole("button", { name: "创建商机" }));

    await waitFor(() => expect(screen.getByRole("button", { name: "创建商机" })).toBeDisabled());
  });
});
