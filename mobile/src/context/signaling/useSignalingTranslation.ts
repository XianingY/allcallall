import { useCallback, useEffect, useRef, useState } from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";
import OnlineTranslationService, {
  type OnlineTranslationResult,
  type OnlineTranslationStatus,
} from "../../services/translation/OnlineTranslationService";
import AnalyticsService from "../../services/AnalyticsService";
import { useSubtitleStore } from "../../store/useSubtitleStore";
import {
  FIRST_TRANSLATION_ENABLED_STORAGE_KEY,
  FIRST_TRANSLATION_HINT_SEEN_STORAGE_KEY,
} from "../../constants/onboarding";
import {
  TRANSLATION_SOURCE_LANG,
  TRANSLATION_TARGET_LANG,
} from "../../config";
import type {
  CallSession,
  CallStatus,
  SignalMessage,
  TranslationInitStatus,
  TranslationMode,
} from "../signalingTypes";
import type { SignalingPeerConnectionResult } from "./useSignalingPeerConnection";

export interface UseSignalingTranslationArgs {
  token: string | null;
  sessionRef: React.MutableRefObject<CallSession | null>;
  statusRef: React.MutableRefObject<CallStatus>;
  status: CallStatus;
  translationRequiresPremium: boolean;
  translationMode: TranslationMode;
  refreshCommercialState: () => void;
  pc: SignalingPeerConnectionResult;
  sendMessage: (message: SignalMessage) => void;
  /**
   * The provider populates this ref from its `resetCallState` so that resetting
   * the whole call also resets translation state (which now lives here). The
   * hook writes the latest reset closure into it on every mount/update.
   */
  translationResetRef: React.MutableRefObject<() => void>;
}

export interface UseSignalingTranslationResult {
  translationEnabled: boolean;
  setTranslationEnabled: React.Dispatch<React.SetStateAction<boolean>>;
  translationLanguage: string;
  setTranslationLanguage: React.Dispatch<React.SetStateAction<string>>;
  translationSourceLanguage: string;
  setTranslationSourceLanguage: React.Dispatch<React.SetStateAction<string>>;
  translationOnlineStatus: OnlineTranslationStatus;
  translationInitStatus: TranslationInitStatus;
  translationInitError: string | null;
  translationPaywallReason: string | null;
  setTranslationPaywallReason: React.Dispatch<React.SetStateAction<string | null>>;
  startOnlinePipeline: () => Promise<boolean>;
  stopOnlinePipeline: () => Promise<void>;
  retryTranslationInitialization: () => Promise<void>;
  toggleTranslation: (enabled: boolean) => Promise<void>;
  pushLocalSubtitle: (
    segmentId: string,
    revision: number,
    isFinal: boolean,
    source: "online",
    original: string,
    translated: string,
    timestamp: number
  ) => void;
  sendFinalSubtitleToPeer: (
    segmentId: string,
    revision: number,
    original: string,
    translated: string,
    timestampMs: number,
    source: "online"
  ) => void;
}

/**
 * Owns all real-time translation state and behaviour: the translation flags,
 * the online pipeline start/stop, the result/error handling, the per-segment
 * subtitle push + peer send, and the two translation effects. `translationMode`
 * / `translationQuotaRemaining` / `translationRequiresPremium` stay with the
 * provider (the provider computes the latter two from commercial state); the
 * provider passes `translationRequiresPremium` in and spreads the returned
 * values into the context value.
 */
export const useSignalingTranslation = ({
  token,
  sessionRef,
  statusRef,
  status,
  translationRequiresPremium,
  translationMode,
  refreshCommercialState,
  pc,
  sendMessage,
  translationResetRef,
}: UseSignalingTranslationArgs): UseSignalingTranslationResult => {
  const [translationEnabled, setTranslationEnabled] = useState<boolean>(false);
  const [translationLanguage, setTranslationLanguage] = useState<string>(TRANSLATION_TARGET_LANG);
  const [translationSourceLanguage, setTranslationSourceLanguage] = useState<string>(TRANSLATION_SOURCE_LANG);
  const [translationOnlineStatus, setTranslationOnlineStatus] = useState<OnlineTranslationStatus>("idle");
  const [translationInitStatus, setTranslationInitStatus] = useState<TranslationInitStatus>("idle");
  const [translationInitError, setTranslationInitError] = useState<string | null>(null);
  const [translationPaywallReason, setTranslationPaywallReason] = useState<string | null>(null);

  const translationStartedTrackedRef = useRef<boolean>(false);
  const { subtitlesDataChannelRef } = pc;

  const stopOnlinePipeline = useCallback(async () => {
    await OnlineTranslationService.stop();
    setTranslationOnlineStatus("idle");
  }, []);

  const pushLocalSubtitle = useCallback((
    segmentId: string,
    revision: number,
    isFinal: boolean,
    source: "online",
    original: string,
    translated: string,
    timestamp: number
  ) => {
    if (!original.trim() && !translated.trim()) return;
    useSubtitleStore.getState().upsertSubtitle({
      segmentId,
      revision,
      isFinal,
      source,
      original,
      translated,
      timestamp,
    });
  }, []);

  const sendFinalSubtitleToPeer = useCallback((
    segmentId: string,
    revision: number,
    original: string,
    translated: string,
    timestampMs: number,
    source: "online"
  ) => {
    const current = sessionRef.current;
    if (!current?.peerEmail) return;

    sendMessage({
      type: "call.subtitle",
      call_id: current.callId,
      to: current.peerEmail,
      payload: {
        segment_id: segmentId,
        revision,
        is_final: true,
        original_text: original,
        translated_text: translated,
        timestamp_ms: timestampMs,
        source,
      },
    });

    // 兼容旧版本 DataChannel 接收端，一个版本周期后可删除。
    // Keep DataChannel compatibility for one release cycle.
    const dc = subtitlesDataChannelRef.current;
    if (dc?.readyState === "open") {
      dc.send(JSON.stringify({
        t: "subtitle",
        segmentId,
        revision,
        isFinal: true,
        originalText: original,
        translatedText: translated,
        timestampMs,
      }));
    }
  }, [sendMessage, subtitlesDataChannelRef, sessionRef]);

  const startOnlinePipeline = useCallback(async (): Promise<boolean> => {
    const current = sessionRef.current;
    if (!token || !current) return false;

    setTranslationInitStatus("initializing");
    setTranslationInitError(null);
    setTranslationOnlineStatus("connecting");

    try {
      await OnlineTranslationService.start(
        {
          token,
          callId: current.callId,
          to: current.peerEmail,
          sourceLang: (translationSourceLanguage === "en" ? "en" : "zh"),
          targetLang: (translationLanguage === "zh" ? "zh" : "en"),
          chunkMs: 400,
        },
        {
          onStatus: (nextStatus) => {
            setTranslationOnlineStatus(nextStatus);
            if (nextStatus === "connected") {
              setTranslationInitStatus("ready");
              setTranslationInitError(null);
            }
          },
          onProviderError: (code, message, recoverable) => {
            if (code === "TRANSLATION_QUOTA_EXHAUSTED") {
              setTranslationPaywallReason("基础通话仍可继续使用，仅实时翻译额度已用尽。升级 Premium 后可立即恢复翻译。");
              setTranslationEnabled(false);
              void refreshCommercialState();
              AnalyticsService.track("paywall_viewed", { reason: "translation_quota_exhausted" });
              return;
            }
            setTranslationInitError(`${code}: ${message}`);
            if (!recoverable) {
              setTranslationInitStatus("failed");
              setTranslationOnlineStatus("error");
            }
          },
          onResult: (result: OnlineTranslationResult) => {
            if (!translationStartedTrackedRef.current) {
              translationStartedTrackedRef.current = true;
              AnalyticsService.track("translation_started");
            }
            const originalText = result.originalText.trim();
            const translatedText = result.translatedText.trim();
            pushLocalSubtitle(
              result.segmentId,
              result.revision,
              result.isFinal,
              "online",
              originalText,
              translatedText,
              result.timestampMs
            );

            if (result.isFinal) {
              sendFinalSubtitleToPeer(
                result.segmentId,
                result.revision,
                originalText,
                translatedText,
                result.timestampMs,
                "online"
              );
            }
          },
        }
      );
      return true;
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setTranslationInitStatus("failed");
      setTranslationInitError(message);
      setTranslationOnlineStatus("error");
      return false;
    }
  }, [pushLocalSubtitle, refreshCommercialState, sendFinalSubtitleToPeer, token, translationLanguage, translationSourceLanguage, sessionRef]);

  const retryTranslationInitialization = useCallback(async () => {
    if (!translationEnabled || statusRef.current !== "in_call") {
      setTranslationInitStatus("idle");
      setTranslationInitError(null);
      return;
    }

    await stopOnlinePipeline();
    await startOnlinePipeline();
  }, [startOnlinePipeline, stopOnlinePipeline, translationEnabled, statusRef]);

  const toggleTranslation = useCallback(async (enabled: boolean) => {
    if (!enabled) {
      translationStartedTrackedRef.current = false;
      setTranslationEnabled(false);
      await stopOnlinePipeline();
      setTranslationInitStatus("idle");
      setTranslationInitError(null);
      setTranslationPaywallReason(null);
      useSubtitleStore.getState().clearSubtitles();
      return;
    }

    if (translationRequiresPremium) {
      setTranslationPaywallReason("基础通话仍可继续使用，仅实时翻译额度已用尽。升级 Premium 后可立即恢复翻译。");
      AnalyticsService.track("paywall_viewed", { reason: "translation_upgrade_required" });
      setTranslationEnabled(false);
      return;
    }

    translationStartedTrackedRef.current = false;
    setTranslationEnabled(true);
    setTranslationInitStatus("idle");
    setTranslationInitError(null);
    setTranslationPaywallReason(null);
    void AsyncStorage.setItem(FIRST_TRANSLATION_ENABLED_STORAGE_KEY, "true");
    void AsyncStorage.setItem(FIRST_TRANSLATION_HINT_SEEN_STORAGE_KEY, "true");
  }, [stopOnlinePipeline, translationRequiresPremium]);

  useEffect(() => {
    if (!translationEnabled || status !== "in_call" || !sessionRef.current?.peerEmail) {
      void stopOnlinePipeline();
      return;
    }

    let cancelled = false;

    const start = async () => {
      const onlineReady = await startOnlinePipeline();
      if (!onlineReady && !cancelled) {
        setTranslationInitStatus("failed");
        setTranslationOnlineStatus("error");
        setTranslationInitError((prev) => prev ?? "online translation start failed");
      }
    };

    void start();
    return () => {
      cancelled = true;
      void stopOnlinePipeline();
    };
  }, [
    startOnlinePipeline,
    status,
    stopOnlinePipeline,
    translationEnabled,
    translationLanguage,
    translationMode,
    translationSourceLanguage,
    sessionRef,
  ]);

  useEffect(() => {
    if (!translationEnabled) return;
    const timer = setInterval(() => {
      useSubtitleStore.getState().pruneExpired();
    }, 500);
    return () => clearInterval(timer);
  }, [translationEnabled]);

  useEffect(() => {
    return () => {
      void OnlineTranslationService.stop();
    };
  }, []);

  // Publish the latest translation-reset closure so the provider's
  // resetCallState can reset translation state alongside the rest of the call.
  useEffect(() => {
    translationResetRef.current = () => {
      translationStartedTrackedRef.current = false;
      setTranslationEnabled(false);
      setTranslationPaywallReason(null);
      setTranslationInitStatus("idle");
      setTranslationInitError(null);
      setTranslationOnlineStatus("idle");
    };
  }, [translationResetRef]);

  return {
    translationEnabled,
    setTranslationEnabled,
    translationLanguage,
    setTranslationLanguage,
    translationSourceLanguage,
    setTranslationSourceLanguage,
    translationOnlineStatus,
    translationInitStatus,
    translationInitError,
    translationPaywallReason,
    setTranslationPaywallReason,
    startOnlinePipeline,
    stopOnlinePipeline,
    retryTranslationInitialization,
    toggleTranslation,
    pushLocalSubtitle,
    sendFinalSubtitleToPeer,
  };
};
