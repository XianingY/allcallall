import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Minimal server-data hook for the mobile app.
 *
 * Every screen used to hand-roll its own `useEffect` + `useState` trio, which
 * is why 23 of 31 screens had no error state at all: forgetting the third
 * piece was invisible until the request failed, and then the screen just went
 * blank. This centralises the three states plus retry so a screen cannot
 * forget one.
 *
 * Deliberately not TanStack Query: the app has 22 service singletons and 31
 * screens, and swapping the data layer wholesale is a much larger change than
 * the problem warrants. This gives the missing pieces (error surface, retry,
 * stale-response guard, unmount safety) without a dependency.
 *
 * Usage:
 *   const rooms = useAsyncData(() => listRooms(token), [token]);
 *   if (rooms.loading) return <Loading />;
 *   if (rooms.error) return <LoadError message={...} onRetry={rooms.reload} />;
 */
export interface AsyncDataResult<T> {
  data: T | null;
  loading: boolean;
  error: Error | null;
  /** Re-runs the loader. Safe to call from a retry button. */
  reload: () => Promise<void>;
}

export interface UseAsyncDataOptions {
  /**
   * Skip loading entirely, e.g. while the token or organization is missing.
   * The hook reports `loading: false` and `data: null` in that state.
   */
  enabled?: boolean;
  /** Keep the previous value visible while reloading instead of clearing it. */
  keepPreviousData?: boolean;
}

export function useAsyncData<T>(
  loader: () => Promise<T>,
  deps: readonly unknown[],
  options: UseAsyncDataOptions = {},
): AsyncDataResult<T> {
  const { enabled = true, keepPreviousData = false } = options;

  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<Error | null>(null);

  // Guards against a slow earlier request overwriting a newer one, and against
  // setting state after unmount.
  const requestId = useRef(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  // Kept in a ref so `reload` stays referentially stable - a loader defined
  // inline changes identity on every render, and having it in the dependency
  // list would re-fetch continuously.
  const loaderRef = useRef(loader);
  loaderRef.current = loader;

  const run = useCallback(async () => {
    const id = requestId.current + 1;
    requestId.current = id;
    setLoading(true);
    setError(null);
    try {
      const result = await loaderRef.current();
      if (!mounted.current || requestId.current !== id) return;
      setData(result);
    } catch (caught) {
      if (!mounted.current || requestId.current !== id) return;
      setError(caught instanceof Error ? caught : new Error(String(caught)));
      if (!keepPreviousData) setData(null);
    } finally {
      if (mounted.current && requestId.current === id) setLoading(false);
    }
  }, [keepPreviousData]);

  useEffect(() => {
    if (!enabled) {
      requestId.current += 1; // cancel anything in flight
      setLoading(false);
      return;
    }
    void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, run, ...deps]);

  const reload = useCallback(async () => {
    if (!enabled) return;
    await run();
  }, [enabled, run]);

  return { data, loading, error, reload };
}
