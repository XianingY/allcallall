import { issueRealtimeTicket, type RealtimeTicket } from "@/api/realtime";
import { runtimeConfig } from "@/lib/runtime-config";

export class TicketSocket<T> {
  private socket: WebSocket | null = null;
  private closed = false;
  private attempt = 0;
  private reconnectTimer: number | null = null;

  constructor(
    private readonly channel: RealtimeTicket["channel"],
    private readonly query: Record<string, string | number> | (() => Record<string, string | number>),
    private readonly onMessage: (message: T) => void,
    private readonly onState: (connected: boolean) => void,
  ) {}

  connect() { this.closed = false; void this.open(); }

  private async open() {
    try {
      const issued = await issueRealtimeTicket(this.channel);
      if (this.closed) return;
      const params = new URLSearchParams({ ticket: issued.ticket });
      const query = typeof this.query === "function" ? this.query() : this.query;
      Object.entries(query).forEach(([key, value]) => params.set(key, String(value)));
      this.socket = new WebSocket(`${runtimeConfig.wsBaseUrl}${issued.websocket_path}?${params}`);
      this.socket.onopen = () => { this.attempt = 0; this.onState(true); };
      this.socket.onmessage = (event) => {
        try { this.onMessage(JSON.parse(String(event.data)) as T); } catch { /* ignore malformed realtime messages */ }
      };
      this.socket.onclose = () => { this.socket = null; this.onState(false); this.scheduleReconnect(); };
      this.socket.onerror = () => this.socket?.close();
    } catch {
      this.onState(false);
      this.scheduleReconnect();
    }
  }

  private scheduleReconnect() {
    if (this.closed || this.reconnectTimer !== null) return;
    const delay = Math.min(30_000, 800 * 2 ** this.attempt) + Math.floor(Math.random() * 250);
    this.attempt += 1;
    this.reconnectTimer = window.setTimeout(() => { this.reconnectTimer = null; void this.open(); }, delay);
  }

  send(message: unknown) {
    if (this.socket?.readyState !== WebSocket.OPEN) return false;
    this.socket.send(JSON.stringify(message));
    return true;
  }

  disconnect() {
    this.closed = true;
    if (this.reconnectTimer !== null) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.socket?.close(1000, "client disconnect");
    this.socket = null;
    this.onState(false);
  }
}
