import { beforeEach, describe, expect, it, vi } from "vitest";

const apiRequestMock = vi.hoisted(() => vi.fn());
vi.mock("@/api/http", () => ({ apiRequest: apiRequestMock }));

import * as collaboration from "@/api/collaboration";

beforeEach(() => {
  apiRequestMock.mockReset();
});

describe("collaboration api", () => {
  it("lists, gets, creates, and updates conversations", async () => {
    await collaboration.listConversations("abc", { limit: 10, offset: 20 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations?filter=abc&limit=10&offset=20");

    apiRequestMock.mockResolvedValue({ conversation: { id: 1 } });
    await expect(collaboration.getConversation(1)).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1");

    await expect(collaboration.createConversation({ type: "channel", title: "T" })).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations", { method: "POST", body: JSON.stringify({ type: "channel", title: "T" }) });

    await expect(collaboration.updateConversation(1, { status: "closed" })).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1", { method: "PATCH", body: JSON.stringify({ status: "closed" }) });
  });

  it("lists and sends messages, including object input and replies", async () => {
    await collaboration.listMessages(1, { beforeId: 5, limit: 25 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages?before_id=5&limit=25");

    await collaboration.listMessages(1);
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages?limit=50");

    apiRequestMock.mockResolvedValue({ message: { id: 9 } });
    await expect(collaboration.sendMessage(1, "hello")).resolves.toEqual({ id: 9 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages", { method: "POST", body: JSON.stringify({ body: "hello", type: "text" }) });

    await collaboration.sendMessage(1, { body: "with file", type: "note", reply_to_message_id: 3 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages", {
      method: "POST",
      body: JSON.stringify({ type: "note", body: "with file", reply_to_message_id: 3 }),
    });
  });

  it("edits, deletes, reacts to, and pins messages", async () => {
    apiRequestMock.mockResolvedValue({ message: { id: 9 } });
    await expect(collaboration.editMessage(1, 9, "new")).resolves.toEqual({ id: 9 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages/9", { method: "PATCH", body: JSON.stringify({ body: "new" }) });

    await expect(collaboration.deleteMessage(1, 9)).resolves.toEqual({ id: 9 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages/9", { method: "DELETE" });

    await expect(collaboration.addReaction(1, 9, "👍")).resolves.toEqual({ id: 9 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages/9/reactions", { method: "POST", body: JSON.stringify({ emoji: "👍" }) });

    await expect(collaboration.removeReaction(1, 9, "👍")).resolves.toEqual({ id: 9 });
    expect(apiRequestMock).toHaveBeenCalledWith(`/conversations/1/messages/9/reactions/${encodeURIComponent("👍")}`, { method: "DELETE" });

    await expect(collaboration.pinMessage(1, 9)).resolves.toEqual({ id: 9 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages/9/pin", { method: "POST" });

    await expect(collaboration.unpinMessage(1, 9)).resolves.toEqual({ id: 9 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/messages/9/pin", { method: "DELETE" });

    apiRequestMock.mockResolvedValue({ messages: [{ id: 9 }] });
    await expect(collaboration.listPinnedMessages(1)).resolves.toEqual([{ id: 9 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/pins");
  });

  it("uploads attachments, sends typing, and marks read", async () => {
    apiRequestMock.mockResolvedValue({ attachment: { id: 4 } });
    const file = new File(["x"], "a.txt", { type: "text/plain" });
    await expect(collaboration.uploadAttachment(1, file)).resolves.toEqual({ id: 4 });
    const [path, init] = apiRequestMock.mock.calls[0];
    expect(path).toBe("/conversations/1/attachments");
    expect(init.method).toBe("POST");
    expect(init.body).toBeInstanceOf(FormData);
    expect(init.body.get("file")).toBe(file);

    await collaboration.sendTyping(1, true);
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/typing", { method: "POST", body: JSON.stringify({ typing: true }) });

    await collaboration.markConversationRead(1);
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/read", { method: "POST" });
  });

  it("manages notes and contacts", async () => {
    apiRequestMock.mockResolvedValue({ notes: [{ id: 1 }] });
    await expect(collaboration.listNotes(1)).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/notes");

    apiRequestMock.mockResolvedValue({ note: { id: 2 } });
    await expect(collaboration.createNote(1, "note")).resolves.toEqual({ id: 2 });
    expect(apiRequestMock).toHaveBeenCalledWith("/conversations/1/notes", { method: "POST", body: JSON.stringify({ body: "note" }) });

    apiRequestMock.mockResolvedValue({ contacts: [{ id: 3 }] });
    await expect(collaboration.listContacts()).resolves.toEqual([{ id: 3 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/users/contacts");

    await collaboration.addContact("a@b.c");
    expect(apiRequestMock).toHaveBeenCalledWith("/users/contacts", { method: "POST", body: JSON.stringify({ email: "a@b.c" }) });

    await collaboration.removeContact(3);
    expect(apiRequestMock).toHaveBeenCalledWith("/users/contacts/3", { method: "DELETE" });

    apiRequestMock.mockResolvedValue({ profile: { id: 3 } });
    await expect(collaboration.getContactProfile(3)).resolves.toEqual({ id: 3 });
    expect(apiRequestMock).toHaveBeenCalledWith("/users/contacts/3/profile");

    const profile = { company: "Acme", role: "Owner" };
    await expect(collaboration.saveContactProfile(3, profile)).resolves.toEqual({ id: 3 });
    expect(apiRequestMock).toHaveBeenCalledWith("/users/contacts/3/profile", { method: "PUT", body: JSON.stringify(profile) });
  });

  it("invitations, follow-ups, call history, and search", async () => {
    apiRequestMock.mockResolvedValue({ invitation: { code: "c1", share_url: "u" } });
    await expect(collaboration.createInvitation({ target_email: "a@b.c" })).resolves.toEqual({ code: "c1", share_url: "u" });
    expect(apiRequestMock).toHaveBeenCalledWith("/invitations", { method: "POST", body: JSON.stringify({ target_email: "a@b.c" }) });

    apiRequestMock.mockResolvedValue({ items: [{ id: 1 }] });
    await expect(collaboration.listFollowUps()).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/follow-ups");

    apiRequestMock.mockResolvedValue({ task: { id: 2 } });
    await expect(collaboration.createFollowUp({ peer_user_id: 3, type: "call", title: "T" })).resolves.toEqual({ id: 2 });
    expect(apiRequestMock).toHaveBeenCalledWith("/follow-ups", {
      method: "POST",
      body: JSON.stringify({ peer_user_id: 3, type: "call", title: "T" }),
    });

    await expect(collaboration.updateFollowUp(2, { status: "done" })).resolves.toEqual({ id: 2 });
    expect(apiRequestMock).toHaveBeenCalledWith("/follow-ups/2", { method: "PATCH", body: JSON.stringify({ status: "done" }) });

    apiRequestMock.mockResolvedValue({ calls: [{ id: 3 }] });
    await expect(collaboration.listCallHistory(7)).resolves.toEqual([{ id: 3 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/calls/history?days=7");

    apiRequestMock.mockResolvedValue({ results: [{ id: "r1" }] });
    await expect(collaboration.searchMessages("hello")).resolves.toEqual([{ id: "r1" }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/search/messages?q=hello");
  });

  it("call followups and deals", async () => {
    await collaboration.fetchCallFollowup("call-1");
    expect(apiRequestMock).toHaveBeenCalledWith("/calls/call-1/followup");

    await collaboration.generateCallFollowup("call-1", true);
    expect(apiRequestMock).toHaveBeenCalledWith("/calls/call-1/followup/regenerate", { method: "POST" });

    await collaboration.generateCallFollowup("call-1");
    expect(apiRequestMock).toHaveBeenCalledWith("/calls/call-1/followup/generate", { method: "POST" });

    apiRequestMock.mockResolvedValue({ pipelines: [{ id: 1 }] });
    await expect(collaboration.listPipelines()).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/pipelines");

    await collaboration.listDeals({ limit: 10, offset: 0 });
    expect(apiRequestMock).toHaveBeenCalledWith("/deals?limit=10&offset=0");

    apiRequestMock.mockResolvedValue({ deal: { id: 5 } });
    await expect(collaboration.getDeal(5)).resolves.toEqual({ id: 5 });
    expect(apiRequestMock).toHaveBeenCalledWith("/deals/5");

    await expect(collaboration.createDeal({ title: "D" })).resolves.toEqual({ id: 5 });
    expect(apiRequestMock).toHaveBeenCalledWith("/deals", { method: "POST", body: JSON.stringify({ title: "D" }) });

    await expect(collaboration.updateDeal(5, { status: "won" })).resolves.toEqual({ id: 5 });
    expect(apiRequestMock).toHaveBeenCalledWith("/deals/5", { method: "PATCH", body: JSON.stringify({ status: "won" }) });

    await collaboration.addDealContact(5, 3);
    expect(apiRequestMock).toHaveBeenCalledWith("/deals/5/contacts", { method: "POST", body: JSON.stringify({ contact_id: 3 }) });

    apiRequestMock.mockResolvedValue({ activities: [{ id: 6 }] });
    await expect(collaboration.listDealActivities(5)).resolves.toEqual([{ id: 6 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/deals/5/activities");
  });
});
