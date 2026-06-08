export interface RemoteTrackLike {
  id: string;
  kind: string;
}

export interface RemoteStreamLike<TTrack extends RemoteTrackLike = RemoteTrackLike> {
  id?: string;
  getTracks(): TTrack[];
  addTrack(track: TTrack): void;
  removeTrack(track: TTrack): void;
}

export interface RemoteStreamRecord<TStream = RemoteStreamLike> {
  id: string;
  stream: TStream;
  participantId?: number;
}

export interface UpsertRemoteStreamInput<
  TStream extends RemoteStreamLike<TTrack>,
  TTrack extends RemoteTrackLike,
> {
  key: string;
  stream: TStream;
  track: TTrack;
  participantId?: number;
}

export class RoomRemoteStreamRegistry<
  TStream extends RemoteStreamLike<TTrack>,
  TTrack extends RemoteTrackLike,
> {
  private readonly streams = new Map<string, TStream>();
  private readonly participantIds = new Map<string, number | undefined>();

  clear(): void {
    this.streams.clear();
    this.participantIds.clear();
  }

  snapshot(): Array<RemoteStreamRecord<TStream>> {
    return Array.from(this.streams.entries()).map(([id, stream]) => ({
      id,
      stream,
      participantId: this.participantIds.get(id),
    }));
  }

  upsert({ key, stream, track, participantId }: UpsertRemoteStreamInput<TStream, TTrack>): void {
    this.participantIds.set(key, participantId);
    const existing = this.streams.get(key);
    if (!existing) {
      this.streams.set(key, stream);
      return;
    }
    if (!existing.getTracks().some((item) => item.id === track.id)) {
      existing.addTrack(track);
    }
  }

  removeTrack(key: string, track: TTrack): void {
    const stream = this.streams.get(key);
    if (!stream) {
      return;
    }
    stream.getTracks()
      .filter((item) => item.id === track.id)
      .forEach((item) => stream.removeTrack(item));
    if (stream.getTracks().length === 0) {
      this.streams.delete(key);
      this.participantIds.delete(key);
    }
  }
}
