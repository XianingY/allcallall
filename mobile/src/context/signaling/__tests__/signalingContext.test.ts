/**
 * Call-flow unit tests for SignalingProvider.
 *
 * These tests exercise the end-to-end call lifecycle (establishment, ICE
 * candidate exchange, and teardown) through the real provider while keeping
 * the heavy native dependencies (react-native-webrtc, transport, media
 * services) behind lightweight mocks. The transport is a controllable fake
 * whose `send` is asserted on directly and whose inbound `message` handler is
 * driven via emitMessage.
 *
 * Placement note: jest's testMatch only includes
 * src/context/signaling/__tests__/**, so this file must stay here.
 */

import React from "react";
import {
  renderHook,
  act,
  type RenderHookResult,
} from "@testing-library/react-hooks";

import { SignalingProvider } from "../../SignalingContext";
import { useSignaling } from "../../signalingContextValue";

type SignalingHookResult = RenderHookResult<unknown, ReturnType<typeof useSignaling>>;

// ---------------------------------------------------------------------------
// react-native-webrtc — each peer connection gets its own fresh jest.fns so we
// can assert on the specific instance created during a test.
// ---------------------------------------------------------------------------
jest.mock("react-native-webrtc", () => ({
  __esModule: true,
  RTCPeerConnection: jest.fn().mockImplementation(() => {
    const pc = {
      remoteDescription: null,
      localDescription: null,
      connectionState: "new",
      iceConnectionState: "new",
      onicecandidate: null,
      ontrack: null,
      onconnectionstatechange: null,
      oniceconnectionstatechange: null,
      ondatachannel: null,
      createOffer: jest.fn().mockResolvedValue({ type: "offer", sdp: "local-offer-sdp" }),
      createAnswer: jest.fn().mockResolvedValue({ type: "answer", sdp: "local-answer-sdp" }),
      setLocalDescription: jest.fn().mockResolvedValue(undefined),
      setRemoteDescription: jest.fn().mockImplementation(function (this: any, d: any) {
        this.remoteDescription = d;
        return Promise.resolve();
      }),
      addIceCandidate: jest.fn().mockResolvedValue(undefined),
      addTrack: jest.fn(),
      addTransceiver: jest.fn(),
      getSenders: jest.fn().mockReturnValue([]),
      getTransceivers: jest.fn().mockReturnValue([]),
      createDataChannel: jest.fn().mockImplementation((label: string) => ({
        label,
        readyState: "connecting",
        send: jest.fn(),
        close: jest.fn(),
      })),
      getStats: jest.fn().mockResolvedValue(new Map()),
      close: jest.fn(),
    };
    (globalThis as any).__mockPCs = (globalThis as any).__mockPCs || [];
    (globalThis as any).__mockPCs.push(pc);
    return pc;
  }),
  RTCSessionDescription: jest.fn(),
  RTCIceCandidate: jest.fn(),
  MediaStream: jest.fn(),
}));

// ---------------------------------------------------------------------------
// Signaling transport — a controllable fake. The provider stores the created
// instance in signalingRef and registers its message handler via `on`, both of
// which we can introspect through jest's mock metadata.
// ---------------------------------------------------------------------------
jest.mock("../../signalingTransports", () => {
  // Singleton transport: the provider recreates the transport whenever its
  // mount-effect dependency chain (localStream -> resetPeerResources ->
  // resetCallState) changes, so returning a stable instance keeps `send` /
  // `on` assertions reliable regardless of how many times the effect re-runs.
  const singleton = {
    connect: jest.fn(),
    disconnect: jest.fn(),
    send: jest.fn().mockReturnValue(true),
    on: jest.fn(),
  };
  (globalThis as any).__mockTransport = singleton;
  return {
    __esModule: true,
    createSignalingTransport: jest.fn(() => singleton),
  };
});

// ---------------------------------------------------------------------------
// Context providers consumed by SignalingProvider.
// ---------------------------------------------------------------------------
jest.mock("../../AuthContext", () => ({
  __esModule: true,
  useAuthContext: () => ({ token: "test-token" }),
}));

jest.mock("../../CommercialContext", () => ({
  __esModule: true,
  useCommercial: () => ({ tier: "premium", usage: [], refreshCommercialState: jest.fn() }),
}));

jest.mock("../../SettingsContext", () => ({
  __esModule: true,
  useSettings: () => ({
    settings: {
      audioNotificationsEnabled: false,
      vibrationEnabled: false,
      defaultAudioEnabled: true,
      defaultVideoEnabled: true,
      cameraFacing: "front",
      videoQuality: "medium",
      videoMaxBitrateKbps: 900,
      videoAdaptiveBitrateEnabled: false,
    },
  }),
}));

// ---------------------------------------------------------------------------
// Config — force E2EE off so the key-exchange branch is skipped.
// ---------------------------------------------------------------------------
jest.mock("../../../config", () => ({
  __esModule: true,
  E2EE_ENABLED: false,
  RESTRICTED_NETWORK_MODE: false,
  SIGNALING_TRANSPORT_MODE: "ws",
  TRANSLATION_MODE: "online",
  TRANSLATION_SOURCE_LANG: "en",
  TRANSLATION_TARGET_LANG: "zh",
}));

jest.mock("../../../api/webrtc", () => ({
  __esModule: true,
  fetchWebRTCConfig: jest.fn().mockResolvedValue({ ice_servers: [] }),
}));

jest.mock("../../../services/VideoService", () => ({
  __esModule: true,
  default: {
    initialize: jest.fn().mockResolvedValue(undefined),
    getLocalStream: jest.fn().mockResolvedValue({
      getTracks: () => [
        { kind: "audio", stop: jest.fn() },
        { kind: "video", stop: jest.fn() },
      ],
      getAudioTracks: () => [{ kind: "audio", stop: jest.fn() }],
      getVideoTracks: () => [{ kind: "video", stop: jest.fn() }],
      addTrack: jest.fn(),
      removeTrack: jest.fn(),
      toURL: () => "fake-stream",
      stop: jest.fn(),
    }),
    toggleAudioTrack: jest.fn(),
    toggleVideoTrack: jest.fn(),
    stopStream: jest.fn(),
    switchCamera: jest.fn(),
    setVideoQuality: jest.fn(),
  },
}));

jest.mock("../../../services/AudioServiceExpo", () => ({
  __esModule: true,
  default: {
    getSpeakerphone: jest.fn().mockReturnValue(false),
    setSpeakerphone: jest.fn().mockResolvedValue(undefined),
    play: jest.fn().mockResolvedValue(undefined),
    stopAll: jest.fn().mockResolvedValue(undefined),
    stop: jest.fn().mockResolvedValue(undefined),
  },
}));

jest.mock("../../../platform/permissionsAdapter", () => ({
  __esModule: true,
  default: {
    requestMeetingPermissions: jest
      .fn()
      .mockResolvedValue({ allGranted: true, camera: true, microphone: true }),
  },
}));

jest.mock("../../../services/VibrationService", () => ({
  __esModule: true,
  default: { vibrate: jest.fn(), cancel: jest.fn() },
}));

jest.mock("../../../services/CameraPermissionService", () => ({
  __esModule: true,
  default: { checkPermissions: jest.fn().mockResolvedValue({ camera: true }) },
}));

jest.mock("../../../services/translation/OnlineTranslationService", () => ({
  __esModule: true,
  default: {
    start: jest.fn().mockResolvedValue(undefined),
    stop: jest.fn().mockResolvedValue(undefined),
  },
}));

jest.mock("../../../services/AnalyticsService", () => ({
  __esModule: true,
  default: { track: jest.fn() },
}));

jest.mock("@react-native-async-storage/async-storage", () => ({
  __esModule: true,
  default: {
    setItem: jest.fn().mockResolvedValue(undefined),
    getItem: jest.fn().mockResolvedValue(null),
    removeItem: jest.fn(),
  },
}));

jest.mock("../../../store/useSubtitleStore", () => ({
  useSubtitleStore: {
    getState: () => ({
      upsertSubtitle: jest.fn(),
      clearSubtitles: jest.fn(),
      pruneExpired: jest.fn(),
    }),
  },
}));

jest.mock("../../../services/e2ee/E2EEKeyExchange", () => ({
  __esModule: true,
  E2EEKeyExchange: jest.fn().mockImplementation(() => ({
    initialize: jest.fn().mockResolvedValue(undefined),
    attachDataChannel: jest.fn(),
    sendPublicKey: jest.fn(),
    destroy: jest.fn(),
  })),
}));

jest.mock("../../../services/e2ee/E2EEService", () => ({
  __esModule: true,
  E2EEUnsupportedError: class extends Error {},
  isE2EECryptoSupported: jest.fn().mockReturnValue(false),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
const makeWrapper = ({ children }: { children: React.ReactNode }) =>
  React.createElement(SignalingProvider, null, children);

const getMessageHandler = (): ((msg: any) => void) | undefined => {
  const transport = (globalThis as any).__mockTransport as any;
  if (!transport) return undefined;
  const onCalls = (transport.on.mock.calls ?? []) as any[];
  const entry = onCalls.find((call) => call[0] === "message");
  return entry?.[1];
};

const getTransport = () => (globalThis as any).__mockTransport;

const getLastPeerConnection = () => {
  const pcs = (globalThis as any).__mockPCs as any[] | undefined;
  return pcs?.[pcs.length - 1];
};

const emitMessage = async (msg: any) => {
  const handler = getMessageHandler();
  if (!handler) return;
  await act(async () => {
    await handler(msg);
  });
};

const INCOMING_INVITE = {
  type: "call.invite",
  call_id: "c1",
  from: "peer@example.com",
  payload: { type: "offer", sdp: "remote-offer-sdp" },
};

const ICE_CANDIDATE = {
  type: "ice.candidate",
  call_id: "c1",
  payload: {
    candidate: "candidate:1 1 udp 2122260223 192.0.2.1 5000 typ host",
    sdpMid: "0",
    sdpMLineIndex: 0,
    iceEpoch: 0,
  },
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------
describe("SignalingProvider call flow", () => {
  let hook: SignalingHookResult;

  beforeEach(() => {
    jest.clearAllMocks();
    (globalThis as any).__mockPCs = [];
    hook = renderHook(() => useSignaling(), { wrapper: makeWrapper });
  });

  it("startCall sends a call.invite with the target email and moves to connecting", async () => {
    await act(async () => {
      await hook.result.current.startCall("peer@example.com");
    });

    expect(getTransport().send).toHaveBeenCalledWith(
      expect.objectContaining({ type: "call.invite", to: "peer@example.com" })
    );
    const inviteCall = (getTransport().send.mock.calls as any[]).find(
      (c) => c[0]?.type === "call.invite"
    );
    expect(inviteCall[0].payload).toMatchObject({ type: "offer" });
    expect(hook.result.current.status).toBe("connecting");
  });

  it("acceptCall answers an incoming invite with call.accept + call.media_update and moves to in_call", async () => {
    await emitMessage(INCOMING_INVITE);
    expect(hook.result.current.session?.direction).toBe("incoming");

    await act(async () => {
      await hook.result.current.acceptCall();
    });

    expect(getTransport().send).toHaveBeenCalledWith(
      expect.objectContaining({ type: "call.accept", call_id: "c1" })
    );
    const acceptCall = (getTransport().send.mock.calls as any[]).find(
      (c) => c[0]?.type === "call.accept"
    );
    expect(acceptCall[0].payload).toMatchObject({ type: "answer" });
    expect(getTransport().send).toHaveBeenCalledWith(
      expect.objectContaining({ type: "call.media_update" })
    );
    expect(hook.result.current.status).toBe("in_call");
  });

  it("applies a remote ICE candidate immediately once the remote description is set", async () => {
    await emitMessage(INCOMING_INVITE);
    await act(async () => {
      await hook.result.current.acceptCall();
    });

    await emitMessage(ICE_CANDIDATE);

    expect(getLastPeerConnection().addIceCandidate).toHaveBeenCalled();
  });

  it("queues a remote ICE candidate before the remote description is applied", async () => {
    await act(async () => {
      await hook.result.current.startCall("peer@example.com");
    });

    await emitMessage(ICE_CANDIDATE);

    // No remote description yet on the outgoing caller's PC, so it must be queued.
    expect(getLastPeerConnection().addIceCandidate).not.toHaveBeenCalled();
  });

  it("rejectCall sends call.reject and resets to idle", async () => {
    await emitMessage(INCOMING_INVITE);
    await act(async () => {
      await hook.result.current.acceptCall();
    });
    expect(hook.result.current.session).not.toBeNull();

    act(() => {
      hook.result.current.rejectCall();
    });

    expect(getTransport().send).toHaveBeenCalledWith(
      expect.objectContaining({ type: "call.reject", call_id: "c1", to: "peer@example.com" })
    );
    expect(hook.result.current.status).toBe("idle");
    expect(hook.result.current.session).toBeNull();
  });

  it("endCall sends call.end and resets to idle", async () => {
    await emitMessage(INCOMING_INVITE);
    await act(async () => {
      await hook.result.current.acceptCall();
    });

    act(() => {
      hook.result.current.endCall();
    });

    expect(getTransport().send).toHaveBeenCalledWith(
      expect.objectContaining({ type: "call.end", call_id: "c1", to: "peer@example.com" })
    );
    expect(hook.result.current.status).toBe("idle");
  });

  it("completes caller establishment when an inbound call.accept arrives", async () => {
    await act(async () => {
      await hook.result.current.startCall("peer@example.com");
    });
    expect(hook.result.current.status).toBe("connecting");

    await emitMessage({
      type: "call.accept",
      call_id: "c1",
      from: "peer@example.com",
      payload: { type: "answer", sdp: "remote-answer-sdp" },
    });

    expect(getLastPeerConnection().setRemoteDescription).toHaveBeenCalled();
    expect(hook.result.current.status).toBe("in_call");
    expect(getTransport().send).toHaveBeenCalledWith(
      expect.objectContaining({ type: "call.media_update" })
    );
  });
});
