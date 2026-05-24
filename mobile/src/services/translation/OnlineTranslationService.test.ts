import OnlineTranslationService, {
  OnlineTranslationResult,
  OnlineTranslationStatus,
  OnlineTranslationStartParams,
} from "./OnlineTranslationService";

// Lightweight pseudo-test helper for environments without Jest wiring.
// Execute manually when needed.
export const buildOnlineTranslationContracts = () => {
  const startParams: OnlineTranslationStartParams = {
    token: "token",
    callId: "call-1",
    to: "peer@example.com",
    sourceLang: "zh",
    targetLang: "en",
    chunkMs: 400,
  };

  const status: OnlineTranslationStatus = "connecting";
  const result: OnlineTranslationResult = {
    sessionId: "session-1",
    segmentId: "seg-1",
    revision: 1,
    isFinal: false,
    originalText: "你好",
    translatedText: "hello",
    timestampMs: Date.now(),
    latencyMs: 500,
  };

  return {
    serviceReadyFlag: OnlineTranslationService.isConnected(),
    startParams,
    status,
    result,
  };
};
