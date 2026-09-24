import { useCallback, useRef, useState } from "react";
import { Alert } from "react-native";
import {
  E2EE_ENABLED,
} from "../../config";
import {
  E2EEUnsupportedError,
  isE2EECryptoSupported,
  type E2EESessionKey,
} from "../../services/e2ee/E2EEService";
import {
  E2EEKeyExchange,
  type E2EEKeyExchangeCallbacks,
  type KeyExchangeRole,
} from "../../services/e2ee/E2EEKeyExchange";
import type { CallSession } from "../signalingTypes";

export interface SignalingE2EEResult {
  e2eeEnabled: boolean;
  e2eeFingerprint: string | null;
  e2eeSessionEstablished: boolean;
  e2eeKeyExchangeRef: React.MutableRefObject<E2EEKeyExchange | null>;
  e2eeDataChannelRef: React.MutableRefObject<any | null>;
  initializeE2EEKeyExchange: (role: KeyExchangeRole) => void;
  resetE2EEState: () => void;
}

/**
 * Owns E2EE key-exchange state and the helper that bootstraps a key exchange
 * once a data channel is available. The shared refs are created here so both the
 * peer-connection reset path and the data-channel open path can reach them.
 */
export const useSignalingE2EE = (
  sessionRef: React.MutableRefObject<CallSession | null>
): SignalingE2EEResult => {
  const [e2eeEnabled, setE2eeEnabled] = useState<boolean>(false);
  const [e2eeFingerprint, setE2eeFingerprint] = useState<string | null>(null);
  const [e2eeSessionEstablished, setE2eeSessionEstablished] = useState<boolean>(false);

  const e2eeKeyExchangeRef = useRef<E2EEKeyExchange | null>(null);
  const e2eeDataChannelRef = useRef<any | null>(null);

  const resetE2EEState = useCallback(() => {
    setE2eeEnabled(false);
    setE2eeFingerprint(null);
    setE2eeSessionEstablished(false);
  }, []);

  const initializeE2EEKeyExchange = useCallback((role: KeyExchangeRole) => {
    const current = sessionRef.current;
    if (!current) return;

    if (!E2EE_ENABLED) {
      setE2eeEnabled(false);
      setE2eeSessionEstablished(false);
      setE2eeFingerprint(null);
      return;
    }

    if (!isE2EECryptoSupported()) {
      console.warn("[E2EE] WebCrypto SubtleCrypto unavailable; skipping E2EE key exchange");
      setE2eeEnabled(false);
      setE2eeSessionEstablished(false);
      setE2eeFingerprint(null);
      return;
    }

    const callbacks: E2EEKeyExchangeCallbacks = {
      onSessionEstablished: (session: E2EESessionKey) => {
        setE2eeFingerprint(session.fingerprint);
        setE2eeSessionEstablished(true);
        console.log(`[E2EE] Session established, fingerprint: ${session.fingerprint.slice(0, 16)}...`);
      },
      onError: (error: Error) => {
        console.error("[E2EE] Key exchange error:", error);
        setE2eeEnabled(false);
        setE2eeSessionEstablished(false);
        setE2eeFingerprint(null);

        if (error instanceof E2EEUnsupportedError) {
          return;
        }

        Alert.alert("E2EE Error", `Failed to establish encrypted session: ${error.message}`);
      },
    };

    const keyExchange = new E2EEKeyExchange(role, current.callId, callbacks);
    e2eeKeyExchangeRef.current = keyExchange;
    setE2eeEnabled(true);

    keyExchange.initialize().then(() => {
      if (e2eeDataChannelRef.current) {
        keyExchange.attachDataChannel(e2eeDataChannelRef.current);
        if (role === "initiator") {
          keyExchange.sendPublicKey();
        }
      }
    });
  }, [sessionRef]);

  return {
    e2eeEnabled,
    e2eeFingerprint,
    e2eeSessionEstablished,
    e2eeKeyExchangeRef,
    e2eeDataChannelRef,
    initializeE2EEKeyExchange,
    resetE2EEState,
  };
};
