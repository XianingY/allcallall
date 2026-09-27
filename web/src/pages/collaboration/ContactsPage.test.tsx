import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { addContactMock, createInvitationMock } = vi.hoisted(() => ({
  addContactMock: vi.fn<(email: string) => Promise<unknown>>(() => new Promise(() => {})),
  createInvitationMock: vi.fn<(input: { target_email: string }) => Promise<unknown>>(
    () => new Promise(() => {}),
  ),
}));

vi.mock("@/api/collaboration", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/collaboration")>()),
  addContact: (email: string) => addContactMock(email),
  createInvitation: (input: { target_email: string }) => createInvitationMock(input),
  listContacts: () => Promise.resolve([]),
}));

vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({
    organizations: [
      { id: 7, name: "Demo", slug: "demo", role: "owner" },
    ],
    activeOrganization: { id: 7, name: "Demo", slug: "demo", role: "owner" },
    loading: false,
    select: () => Promise.resolve(),
    create: () => Promise.resolve({}),
  }),
}));

import { ContactsPage } from "@/pages/collaboration/ContactsPage";

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <ContactsPage />
    </QueryClientProvider>,
  );

describe("ContactsPage double-submit guards", () => {
  afterEach(cleanup);

  it("adds a contact only once for a burst of clicks", async () => {
    addContactMock.mockClear();
    renderPage();

    fireEvent.change(await screen.findByPlaceholderText("通过邮箱添加联系人"), {
      target: { value: "ada@example.com" },
    });
    const button = screen.getByRole("button", { name: /添加/ });
    fireEvent.click(button);
    await waitFor(() => expect(addContactMock).toHaveBeenCalledTimes(1));
    // RQ flushes isPending in a microtask, so re-click only after that render — real browser clicks are separate tasks.
    fireEvent.click(button);
    expect(addContactMock).toHaveBeenCalledTimes(1);
    expect(addContactMock).toHaveBeenCalledWith("ada@example.com");
  });

  it("generates an invitation only once for a burst of clicks", async () => {
    createInvitationMock.mockClear();
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /发送邀请/ }));
    fireEvent.change(await screen.findByLabelText("邮箱"), { target: { value: "ada@example.com" } });
    const button = await screen.findByRole("button", { name: /生成邀请/ });
    fireEvent.click(button);
    await waitFor(() => expect(createInvitationMock).toHaveBeenCalledTimes(1));
    // RQ flushes isPending in a microtask, so re-click only after that render — real browser clicks are separate tasks.
    fireEvent.click(button);
    expect(createInvitationMock).toHaveBeenCalledTimes(1);
    expect(createInvitationMock).toHaveBeenCalledWith({ target_email: "ada@example.com" });
  });
});
