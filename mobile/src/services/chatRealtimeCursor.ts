export interface ChatEventPayload {
  event_id?: number;
  event: string;
  organization_id: number;
  payload: unknown;
  created_at?: string;
}

export class ChatRealtimeCursor {
  private readonly seenEvents = new Map<string, number>();
  private readonly lastEventIds = new Map<number, number>();

  constructor(private readonly dedupeWindowMs = 5_000) {}

  getSinceId(organizationId: number) {
    return this.lastEventIds.get(organizationId) ?? 0;
  }

  shouldSkip(payload: ChatEventPayload, now = Date.now()) {
    this.prune(now);
    if (payload.event_id) {
      const signature = `${payload.organization_id}:${payload.event_id}`;
      const seenAt = this.seenEvents.get(signature);
      if (seenAt && now - seenAt <= this.dedupeWindowMs) {
        return true;
      }
      this.seenEvents.set(signature, now);
      const current = this.lastEventIds.get(payload.organization_id) ?? 0;
      if (payload.event_id > current) {
        this.lastEventIds.set(payload.organization_id, payload.event_id);
      }
      return false;
    }

    const signature = JSON.stringify({
      event: payload.event,
      organization_id: payload.organization_id,
      payload: payload.payload,
    });
    const seenAt = this.seenEvents.get(signature);
    if (seenAt && now - seenAt <= this.dedupeWindowMs) {
      return true;
    }
    this.seenEvents.set(signature, now);
    return false;
  }

  private prune(now: number) {
    for (const [key, timestamp] of this.seenEvents.entries()) {
      if (now - timestamp > this.dedupeWindowMs) {
        this.seenEvents.delete(key);
      }
    }
  }
}
