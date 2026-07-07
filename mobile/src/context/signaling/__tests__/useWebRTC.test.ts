import { renderHook, act } from '@testing-library/react-hooks';
import { useWebRTC } from '../useWebRTC';

// Mock react-native-webrtc
jest.mock('react-native-webrtc', () => ({
  RTCPeerConnection: jest.fn(() => ({
    createOffer: jest.fn().mockResolvedValue({ type: 'offer', sdp: 'mock-sdp' }),
    createAnswer: jest.fn().mockResolvedValue({ type: 'answer', sdp: 'mock-sdp' }),
    setLocalDescription: jest.fn().mockResolvedValue(undefined),
    setRemoteDescription: jest.fn().mockResolvedValue(undefined),
    addIceCandidate: jest.fn().mockResolvedValue(undefined),
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    close: jest.fn(),
    getStats: jest.fn().mockResolvedValue(new Map()),
  })),
  RTCSessionDescription: jest.fn(),
  RTCIceCandidate: jest.fn(),
  MediaStream: jest.fn(),
}));

describe('useWebRTC', () => {
  it('creates peer connection on mount', () => {
    const { result } = renderHook(() => useWebRTC({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      onOfferCreated: jest.fn(),
      onAnswerCreated: jest.fn(),
      onIceCandidate: jest.fn(),
    }));

    expect(result.current.peerConnection).toBeDefined();
    expect(result.current.localStream).toBeNull();
    expect(result.current.remoteStream).toBeNull();
  });

  it('creates offer successfully', async () => {
    const onOfferCreated = jest.fn();
    
    const { result } = renderHook(() => useWebRTC({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      onOfferCreated,
      onAnswerCreated: jest.fn(),
      onIceCandidate: jest.fn(),
    }));

    await act(async () => {
      await result.current.createOffer();
    });

    expect(onOfferCreated).toHaveBeenCalledWith({
      type: 'offer',
      sdp: 'mock-sdp',
    });
  });

  it('cleans up on unmount', () => {
    const { result, unmount } = renderHook(() => useWebRTC({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      onOfferCreated: jest.fn(),
      onAnswerCreated: jest.fn(),
      onIceCandidate: jest.fn(),
    }));

    const peerConnection = result.current.peerConnection;
    
    unmount();

    expect(peerConnection.close).toHaveBeenCalled();
  });
});
