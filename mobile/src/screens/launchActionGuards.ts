/**
 * Pure guards for the launch flows that used to fail silently.
 *
 * Deliberately free of react-native imports: these helpers are exercised by
 * the node test runner (`npm run test:unit`), which cannot load RN modules.
 */

/**
 * Result of a guarded action. Failures become `{ status: "error" }` instead of
 * escaping as an unhandled promise rejection, and a tap that arrives while a
 * previous invocation is still in flight becomes `{ status: "busy" }` rather
 * than starting a second request.
 */
export type SingleFlightResult<T> =
  | { status: "ok"; value: T }
  | { status: "busy" }
  | { status: "error"; error: Error };

export interface SingleFlight {
  /** Synchronous busy check, safe to call before any `await`. */
  isBusy(): boolean;
  run<T>(action: () => Promise<T>): Promise<SingleFlightResult<T>>;
}

const toError = (value: unknown): Error =>
  value instanceof Error ? value : new Error(String(value));

/**
 * Runs at most one action at a time. Use one instance per interaction
 * (回拨 button, send button) held in a `useRef` so the guard stays synchronous
 * across renders.
 */
export const createSingleFlight = (): SingleFlight => {
  let busy = false;
  return {
    isBusy: () => busy,
    run: async <T>(action: () => Promise<T>): Promise<SingleFlightResult<T>> => {
      if (busy) {
        return { status: "busy" };
      }
      busy = true;
      try {
        return { status: "ok", value: await action() };
      } catch (error) {
        return { status: "error", error: toError(error) };
      } finally {
        busy = false;
      }
    },
  };
};

/**
 * What a data screen should render once both the organization list and the
 * screen's own fetch have settled.
 *
 * The organization error always wins: `currentOrganization` stays null when
 * that request fails, every fetch is gated on it, and without this the screen
 * would render its "当前工作区还没有会议。" empty copy over a network failure -
 * turning a broken workspace list into what looks like an empty account.
 */
export type RoomsLoadView =
  | { kind: "organization-error"; message: string }
  | { kind: "list-error"; message: string }
  | { kind: "empty" }
  | { kind: "content" };

export interface RoomsLoadInput {
  organizationError: Error | null;
  listError: string | null;
  hasRooms: boolean;
}

export const resolveRoomsLoadView = (input: RoomsLoadInput): RoomsLoadView => {
  if (input.organizationError) {
    return {
      kind: "organization-error",
      message: input.organizationError.message || "无法加载工作区列表",
    };
  }
  if (input.listError) {
    return { kind: "list-error", message: input.listError };
  }
  if (!input.hasRooms) {
    return { kind: "empty" };
  }
  return { kind: "content" };
};
