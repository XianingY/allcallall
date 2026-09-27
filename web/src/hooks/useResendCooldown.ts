import { useCallback, useEffect, useState } from "react";

/**
 * Rate limits a repeatable action such as "send verification code".
 *
 * Mobile already had a 60 second countdown; web had none, so clicking the
 * button repeatedly sent one email per click. That is both harassment of the
 * address owner and a way to burn through the backend's send quota.
 */
export function useResendCooldown(seconds = 60) {
  const [remaining, setRemaining] = useState(0);

  useEffect(() => {
    if (remaining <= 0) {
      return;
    }
    const timer = window.setTimeout(() => setRemaining((value) => value - 1), 1000);
    return () => window.clearTimeout(timer);
  }, [remaining]);

  const start = useCallback(() => setRemaining(seconds), [seconds]);

  return { remaining, start, coolingDown: remaining > 0 };
}
