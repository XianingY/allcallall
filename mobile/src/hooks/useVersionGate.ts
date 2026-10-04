import { useCallback, useEffect, useState } from "react";

import { checkAppVersion } from "../api/appVersion";
import { decideVersionGate, type VersionGateState } from "../versionGate";

export type { VersionGateState };
export { decideVersionGate };

// How long to hold the app before letting the user in regardless. Long enough
// that a normal round-trip finishes inside it and nobody sees a gate appear
// after the UI, short enough that a slow network does not read as a broken app.
const CHECK_TIMEOUT_MS = 2500;

export const useVersionGate = (): { state: VersionGateState; recheck: () => void } => {
  const [state, setState] = useState<VersionGateState>({ status: "checking" });

  const run = useCallback(async () => {
    setState({ status: "checking" });

    // The gate is a courtesy, not a lock: if the check cannot complete in time we
    // let the user through and keep the app usable.
    let settled = false;
    const timeout = setTimeout(() => {
      if (settled) return;
      settled = true;
      console.warn("[VersionGate] check timed out, allowing the app to continue");
      setState({ status: "unknown", reason: "timeout" });
    }, CHECK_TIMEOUT_MS);

    try {
      const check = await checkAppVersion();
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      setState(decideVersionGate(check));
    } catch (error) {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      const reason = error instanceof Error ? error.message : "version check failed";
      console.warn("[VersionGate] check failed, allowing the app to continue:", error);
      setState({ status: "unknown", reason });
    }
  }, []);

  useEffect(() => {
    void run();
  }, [run]);

  return { state, recheck: run };
};
