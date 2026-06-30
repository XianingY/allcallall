export type SignalingState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting' | 'error';

export interface SignalingEvent {
  type: string;
  payload?: any;
}

export type Listener = (state: SignalingState, event?: SignalingEvent) => void;

export class SignalingMachine {
  private state: SignalingState = 'disconnected';
  private listeners: Set<Listener> = new Set();
  private ws: WebSocket | null = null;
  private peerConnection: RTCPeerConnection | null = null;

  constructor(private url: string, private token: string) {}

  public subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    listener(this.state);
    return () => this.listeners.delete(listener);
  }

  private notify(event?: SignalingEvent) {
    this.listeners.forEach((listener) => listener(this.state, event));
  }

  public connect() {
    if (this.state === 'connected' || this.state === 'connecting') return;
    this.state = 'connecting';
    this.notify();

    this.ws = new WebSocket(`${this.url}?token=${this.token}`);

    this.ws.onopen = () => {
      this.state = 'connected';
      this.notify();
    };

    this.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        this.handleProtocolMessage(msg);
      } catch (err) {
        console.error('Failed to parse signaling message', err);
      }
    };

    this.ws.onclose = () => {
      this.state = 'disconnected';
      this.notify();
    };

    this.ws.onerror = () => {
      this.state = 'error';
      this.notify({ type: 'WS_ERROR' });
    };
  }

  private handleProtocolMessage(msg: any) {
    // Abstract protocol handling (offer/answer/candidate)
    this.notify({ type: 'MESSAGE_RECEIVED', payload: msg });
  }

  public send(msg: any) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    } else {
      console.warn('Cannot send message, WS not open');
    }
  }

  public disconnect() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.state = 'disconnected';
    this.notify();
  }
}
