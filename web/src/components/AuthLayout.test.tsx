import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { AuthLayout } from "@/components/AuthLayout";

describe("AuthLayout", () => {
  afterEach(cleanup);

  it("presents a static realtime collaboration example without false step markers", () => {
    render(
      <MemoryRouter>
        <AuthLayout title="登录工作台" description="使用 AllCallAll 账号继续">
          <form />
        </AuthLayout>
      </MemoryRouter>,
    );

    expect(screen.queryByText("01")).not.toBeInTheDocument();
    expect(screen.queryByText("02")).not.toBeInTheDocument();

    const activity = screen.getByRole("list", { name: "一次会话如何形成记录" });
    const items = within(activity).getAllByRole("listitem");
    expect(items).toHaveLength(4);
    expect(within(activity).getByText("客户经理 · 提问")).toBeInTheDocument();
    expect(within(activity).getByText("Agent · 总结")).toBeInTheDocument();
    expect(within(activity).getByText("队友 · 审批")).toBeInTheDocument();
    expect(within(activity).getByText("工作台 · 归档")).toBeInTheDocument();
  });
});
