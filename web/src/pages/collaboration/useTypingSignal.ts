import { useCallback, useEffect, useRef } from "react";

import { sendTyping } from "@/api/collaboration";

export const TYPING_REFRESH_MS = 2500;
export const TYPING_STOP_MS = 1200;

type TypingSignalInput = {
  conversationId: number | null;
  enabled?: boolean;
  onBeforeStop?: () => void;
};

export function useTypingSignal({
  conversationId,
  enabled = true,
  onBeforeStop,
}: TypingSignalInput) {
  const activeRef = useRef(false);
  const failureLoggedRef = useRef(false);
  const stopTimerRef = useRef<number | null>(null);
  const refreshTimerRef = useRef<number | null>(null);
  const onBeforeStopRef = useRef(onBeforeStop);

  useEffect(() => {
    onBeforeStopRef.current = onBeforeStop;
  }, [onBeforeStop]);

  const clearTimers = useCallback(() => {
    if (stopTimerRef.current !== null) {
      window.clearTimeout(stopTimerRef.current);
      stopTimerRef.current = null;
    }
    if (refreshTimerRef.current !== null) {
      window.clearInterval(refreshTimerRef.current);
      refreshTimerRef.current = null;
    }
  }, []);

  const sendSignal = useCallback(
    (typing: boolean) => {
      if (!conversationId) return;
      void sendTyping(conversationId, typing).catch((error: unknown) => {
        if (failureLoggedRef.current) return;
        failureLoggedRef.current = true;
        console.error("[useTypingSignal] sendTyping failed", error);
      });
    },
    [conversationId],
  );

  const stopTyping = useCallback(() => {
    if (!activeRef.current) return;
    clearTimers();
    activeRef.current = false;
    onBeforeStopRef.current?.();
    sendSignal(false);
  }, [clearTimers, sendSignal]);

  const signalTyping = useCallback((next: string) => {
    if (!enabled || !conversationId || !next.trim()) return;

    if (!activeRef.current) {
      activeRef.current = true;
      failureLoggedRef.current = false;
      sendSignal(true);
      refreshTimerRef.current = window.setInterval(() => {
        sendSignal(true);
      }, TYPING_REFRESH_MS);
    }

    if (stopTimerRef.current !== null) {
      window.clearTimeout(stopTimerRef.current);
    }
    stopTimerRef.current = window.setTimeout(() => {
      stopTyping();
    }, TYPING_STOP_MS);
  }, [conversationId, enabled, sendSignal, stopTyping]);

  useEffect(() => {
    if (!enabled) stopTyping();
    return stopTyping;
  }, [enabled, stopTyping]);

  return { signalTyping, stopTyping };
}
