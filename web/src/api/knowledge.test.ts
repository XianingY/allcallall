import { beforeEach, describe, expect, it, vi } from "vitest";

const apiRequestMock = vi.hoisted(() => vi.fn());
vi.mock("@/api/http", () => ({ apiRequest: apiRequestMock }));

import * as knowledge from "@/api/knowledge";

beforeEach(() => {
  apiRequestMock.mockReset();
});

describe("knowledge api", () => {
  it("lists sources, groups, duplicates, and dead letters", async () => {
    apiRequestMock.mockResolvedValue({ sources: [{ id: 1 }] });
    await expect(knowledge.listKnowledgeSources()).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/sources");

    apiRequestMock.mockResolvedValue({ source_groups: [{ id: 2 }] });
    await expect(knowledge.listSourceGroups()).resolves.toEqual([{ id: 2 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/source-groups");

    apiRequestMock.mockResolvedValue({ duplicate_candidates: [{ id: 3 }] });
    await expect(knowledge.listDuplicates()).resolves.toEqual([{ id: 3 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/duplicate-candidates");

    apiRequestMock.mockResolvedValue({ dead_letters: [{ id: 4 }] });
    await expect(knowledge.listDeadLetters()).resolves.toEqual([{ id: 4 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/dead-letters");
  });

  it("gets one source", async () => {
    apiRequestMock.mockResolvedValue({ id: 7 });
    await expect(knowledge.getKnowledgeSource(7)).resolves.toEqual({ id: 7 });
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/sources/7");
  });

  it("creates text and URL sources", async () => {
    apiRequestMock.mockResolvedValue({ source: { id: 1 } });
    await expect(knowledge.createTextSource("Notes", "hello", 5)).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/sources", {
      method: "POST",
      body: JSON.stringify({ kind: "manual_text", title: "Notes", text: "hello", conversation_id: 5 }),
    });

    await knowledge.createURLSource("Docs", "https://example.com");
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/sources", {
      method: "POST",
      body: JSON.stringify({ kind: "url", title: "Docs", url: "https://example.com", conversation_id: undefined }),
    });
  });

  it("creates file sources as multipart", async () => {
    apiRequestMock.mockResolvedValue({ source: { id: 2 } });
    const file = new File(["body"], "notes.txt", { type: "text/plain" });
    await knowledge.createFileSource("File", file, 3);
    const [path, init] = apiRequestMock.mock.calls[0];
    expect(path).toBe("/knowledge/sources");
    expect(init.method).toBe("POST");
    expect(init.body).toBeInstanceOf(FormData);
    expect(init.body.get("kind")).toBe("file");
    expect(init.body.get("title")).toBe("File");
    expect(init.body.get("file")).toBe(file);
    expect(init.body.get("conversation_id")).toBe("3");
  });

  it("reingests, sets canonical, decides duplicates, and retries dead letters", async () => {
    await knowledge.reingestSource(11);
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/sources/11/reingest", { method: "POST", body: "{}" });

    await knowledge.setCanonicalSource(12, 13);
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/source-groups/12/canonical", {
      method: "POST",
      body: JSON.stringify({ source_id: 13 }),
    });

    await knowledge.decideDuplicate(14, "reject");
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/duplicate-candidates/14/decision", {
      method: "POST",
      body: JSON.stringify({ decision: "reject" }),
    });

    await knowledge.retryDeadLetter(15);
    expect(apiRequestMock).toHaveBeenCalledWith("/knowledge/dead-letters/15/retry", { method: "POST", body: "{}" });
  });
});
