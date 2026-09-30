import { describe, expect, it, vi } from "vitest";

import { APIError, markSessionExpired } from "@/api/http";
import { createQueryClient } from "@/api/queryClient";

/** A 401 produced by http.ts's refresh-exhaustion path: the refresh cookie is gone. */
const sessionExpired401 = () => {
  const error = new APIError(401, "REFRESH_FAILED", "refresh token is no longer valid");
  markSessionExpired(error);
  return error;
};

/** A plain 401, e.g. one forbidden request that must not kill the session. */
const plain401 = () => new APIError(401, "FORBIDDEN", "not allowed");

/** Flush a macrotask so trailing cache notifications are observable. */
const flush = () => new Promise<void>((resolve) => { setTimeout(resolve, 0); });

describe("createQueryClient session-expiry bridge", () => {
  it("ends the session when a query rejects with a session-expired 401", async () => {
    const endSession = vi.fn();
    const client = createQueryClient({ endSession });
    const error = sessionExpired401();

    await expect(client.fetchQuery({ queryKey: ["inbox"], queryFn: () => Promise.reject(error), retry: false })).rejects.toBe(error);
    await flush();

    expect(endSession).toHaveBeenCalledTimes(1);
    client.clear();
  });

  it("ends the session exactly once when parallel queries all reject with session-expired 401s", async () => {
    const endSession = vi.fn();
    const client = createQueryClient({ endSession });

    const attempts = Array.from({ length: 10 }, (_, index) =>
      client.fetchQuery({ queryKey: ["burst", index], queryFn: () => Promise.reject(sessionExpired401()), retry: false }));
    const results = await Promise.allSettled(attempts);
    await flush();

    expect(results.every((result) => result.status === "rejected")).toBe(true);
    expect(endSession).toHaveBeenCalledTimes(1);
    client.clear();
  });

  it("does not end the session for a plain 401 without the session-expired marker", async () => {
    const endSession = vi.fn();
    const client = createQueryClient({ endSession });
    const error = plain401();

    await expect(client.fetchQuery({ queryKey: ["forbidden"], queryFn: () => Promise.reject(error), retry: false })).rejects.toBe(error);
    await flush();

    expect(endSession).not.toHaveBeenCalled();
    client.clear();
  });

  it("ends the session when a mutation rejects with a session-expired 401", async () => {
    const endSession = vi.fn();
    const client = createQueryClient({ endSession });
    const error = sessionExpired401();

    const mutation = client.getMutationCache().build(client, { mutationFn: () => Promise.reject(error), retry: false });
    await expect(mutation.execute({})).rejects.toBe(error);
    await flush();

    expect(endSession).toHaveBeenCalledTimes(1);
    client.clear();
  });
});
