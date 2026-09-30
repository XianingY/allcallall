import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";

import { isSessionExpiredError } from "@/api/http";

/**
 * The slice of auth state the query layer needs. AuthProvider registers its
 * logout path here so a refresh-exhausted 401 can reuse the existing
 * status → anonymous → ProtectedRoute redirect machinery instead of inventing
 * a second way to reach the login screen.
 */
export interface AuthBridge {
  endSession: () => void | Promise<void>;
}

/**
 * One refresh attempt fails once, but every in-flight query/mutation rejects
 * with that same dead-session 401 in the same tick. Collapse the burst into a
 * single endSession call, while a cooldown (not a permanent latch) keeps a
 * later re-login able to expire again.
 */
const SESSION_END_DEDUP_MS = 1_000;

export function createQueryClient(authBridge: AuthBridge): QueryClient {
  let lastEndSessionAt = 0;
  const endSessionOnce = () => {
    const now = Date.now();
    if (now - lastEndSessionAt < SESSION_END_DEDUP_MS) return;
    lastEndSessionAt = now;
    void Promise.resolve(authBridge.endSession()).catch(() => undefined);
  };
  const onError = (error: unknown) => {
    if (isSessionExpiredError(error)) endSessionOnce();
  };

  return new QueryClient({
    defaultOptions: {
      queries: { staleTime: 15_000, retry: 1, refetchOnWindowFocus: false },
      mutations: { retry: 0 },
    },
    queryCache: new QueryCache({ onError }),
    mutationCache: new MutationCache({ onError }),
  });
}
