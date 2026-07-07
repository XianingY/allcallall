// mobile/src/api/__tests__/signaling.test.ts
import { SignalingClient } from '../signaling';

// Mock WebSocket
const mockWsOn = jest.fn();
const mockWsSend = jest.fn();
const mockWsClose = jest.fn();

global.WebSocket = jest.fn().mockImplementation(() => ({
  on: mockWsOn,
  send: mockWsSend,
  close: mockWsClose,
  readyState: 1,
})) as any;

describe('SignalingClient Exponential Backoff', () => {
  let client: SignalingClient;
  
  beforeEach(() => {
    jest.useFakeTimers();
    client = new SignalingClient('test-token');
  });
  
  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
  });
  
  it('uses exponential backoff on reconnect', () => {
    const connectSpy = jest.spyOn(client as any, 'openSocket');
    
    // Simulate connection
    client.connect();
    
    // Get the ws instance
    const ws = (client as any).ws;
    
    // Simulate disconnect
    ws.onclose({ code: 1000, reason: 'test' });
    
    // First reconnect attempt (~1s delay, with jitter)
    jest.advanceTimersByTime(1200);
    expect(connectSpy).toHaveBeenCalledTimes(2);
    
    // Second disconnect
    (client as any).ws.onclose({ code: 1000, reason: 'test' });
    
    // Second reconnect attempt (~2s delay)
    jest.advanceTimersByTime(2400);
    expect(connectSpy).toHaveBeenCalledTimes(3);
  });
});
