import { renderHook, act } from '@testing-library/react-hooks';
import { useWebRTC } from '../useWebRTC';

const mockClose = jest.fn();
const mockCreateOffer = jest.fn().mockResolvedValue({ type: 'offer', sdp: 'mock-sdp' });
const mockSetLocalDescription = jest.fn().mockResolvedValue(undefined);
const mockAddEventListener = jest.fn();
const mockRemoveEventListener = jest.fn();

jest.mock('react-native-webrtc', () => ({
  RTCPeerConnection: jest.fn().mockImplementation(() => ({
    createOffer: mockCreateOffer,
    createAnswer: jest.fn().mockResolvedValue({ type: 'answer', sdp: 'mock-sdp' }),
    setLocalDescription: mockSetLocalDescription,
    setRemoteDescription: jest.fn().mockResolvedValue(undefined),
    addIceCandidate: jest.fn().mockResolvedValue(undefined),
    addEventListener: mockAddEventListener,
    removeEventListener: mockRemoveEventListener,
    close: mockClose,
    getStats: jest.fn().mockResolvedValue(new Map()),
  })),
  RTCSessionDescription: jest.fn(),
  RTCIceCandidate: jest.fn(),
  MediaStream: jest.fn(),
}));

const baseConfig = {
  iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
  onOfferCreated: jest.fn(),
  onAnswerCreated: jest.fn(),
  onIceCandidate: jest.fn(),
  onRemoteStream: jest.fn(),
  onConnectionStateChange: jest.fn(),
};

describe('useWebRTC', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('creates a peer connection on mount', () => {
    const { RTCPeerConnection } = require('react-native-webrtc');
    renderHook(() => useWebRTC(baseConfig));

    expect(RTCPeerConnection).toHaveBeenCalledTimes(1);
    expect(mockAddEventListener).toHaveBeenCalledWith('icecandidate', expect.any(Function));
    expect(mockAddEventListener).toHaveBeenCalledWith('track', expect.any(Function));
  });

  it('creates an offer and forwards it to onOfferCreated', async () => {
    const { result } = renderHook(() => useWebRTC(baseConfig));

    await act(async () => {
      await result.current.createOffer();
    });

    expect(mockCreateOffer).toHaveBeenCalled();
    expect(mockSetLocalDescription).toHaveBeenCalledWith({ type: 'offer', sdp: 'mock-sdp' });
    expect(baseConfig.onOfferCreated).toHaveBeenCalledWith({ type: 'offer', sdp: 'mock-sdp' });
  });

  it('cleans up the peer connection on unmount', () => {
    const { unmount } = renderHook(() => useWebRTC(baseConfig));

    unmount();

    expect(mockClose).toHaveBeenCalled();
  });
});
