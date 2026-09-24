import OnlineTranslationService, {
  OnlineTranslationResult,
  OnlineTranslationStatus,
  OnlineTranslationStartParams,
} from "./OnlineTranslationService";

describe("OnlineTranslationService contract", () => {
  const startParams: OnlineTranslationStartParams = {
    token: "token",
    callId: "call-1",
    to: "peer@example.com",
    sourceLang: "zh",
    targetLang: "en",
    chunkMs: 400,
  };

  it("reports a not-connected boolean state before start", () => {
    expect(typeof OnlineTranslationService.isConnected()).toBe("boolean");
    expect(OnlineTranslationService.isConnected()).toBe(false);
  });

  it("accepts a well-formed start/status/result payload", () => {
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

    expect(status).toBe("connecting");
    expect(result.translatedText).toBe("hello");
    expect(result.segmentId).toBe("seg-1");
    expect(startParams.sourceLang).toBe("zh");
    expect(startParams.targetLang).toBe("en");
  });
});
