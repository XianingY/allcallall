import { beforeEach, describe, expect, it, vi } from "vitest";

const apiRequestMock = vi.hoisted(() => vi.fn());
const apiDownloadMock = vi.hoisted(() => vi.fn());
vi.mock("@/api/http", () => ({ apiRequest: apiRequestMock, apiDownload: apiDownloadMock }));

import * as meetings from "@/api/meetings";

beforeEach(() => {
  apiRequestMock.mockReset();
  apiDownloadMock.mockReset();
});

describe("meetings api", () => {
  it("lists and creates rooms", async () => {
    apiRequestMock.mockResolvedValue({ rooms: [{ id: 1 }] });
    await expect(meetings.listRooms()).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms");

    apiRequestMock.mockResolvedValue({ room: { id: 2 } });
    await expect(meetings.createRoom({ title: "Standup" })).resolves.toEqual({ id: 2 });
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms", {
      method: "POST",
      body: JSON.stringify({ title: "Standup" }),
    });
  });

  it("gets room state, joins, and leaves", async () => {
    apiRequestMock.mockResolvedValue({ room: { id: 1 } });
    await expect(meetings.getRoom(1)).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/state");

    await expect(meetings.joinRoom(1)).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/join", { method: "POST" });

    await expect(meetings.leaveRoom(1)).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/leave", { method: "POST" });
  });

  it("sends offer, ICE, renegotiation, and media updates", async () => {
    await meetings.sendRoomOffer(1, "sdp");
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/offer", { method: "POST", body: JSON.stringify({ sdp: "sdp" }) });

    const candidate = { candidate: "c", sdpMid: "0", sdpMLineIndex: 0 };
    await meetings.sendRoomICE(1, candidate);
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/ice", { method: "POST", body: JSON.stringify(candidate) });

    await meetings.sendRoomRenegotiationAnswer(1, "answer");
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/renegotiate", { method: "POST", body: JSON.stringify({ sdp: "answer" }) });

    await meetings.updateRoomMedia(1, { audio_enabled: false });
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/media", { method: "POST", body: JSON.stringify({ audio_enabled: false }) });
  });

  it("starts and stops recordings", async () => {
    apiRequestMock.mockResolvedValue({ recording: { id: 8 } });
    await expect(meetings.startRecording(1)).resolves.toEqual({ id: 8 });
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/recording/start", { method: "POST" });

    await expect(meetings.stopRecording(1)).resolves.toEqual({ id: 8 });
    expect(apiRequestMock).toHaveBeenCalledWith("/rooms/1/recording/stop", { method: "POST" });
  });

  it("lists recordings with pagination and fetches one", async () => {
    apiRequestMock.mockResolvedValue({ recordings: [], pagination: { total: 0, limit: 20, offset: 0, has_more: false } });
    await meetings.listRecordings({ limit: 20, offset: 40 });
    expect(apiRequestMock).toHaveBeenCalledWith("/recordings?limit=20&offset=40");

    apiRequestMock.mockResolvedValue({ recording: { id: 9 } });
    await expect(meetings.getRecording(9)).resolves.toEqual({ id: 9 });
    expect(apiRequestMock).toHaveBeenCalledWith("/recordings/9");
  });

  it("fetches transcripts, retries transcription, and downloads files", async () => {
    await meetings.getTranscript(9, 42);
    expect(apiRequestMock).toHaveBeenCalledWith("/recordings/9/transcript?limit=100&after_id=42");

    apiRequestMock.mockResolvedValue({ transcription: { id: 10 } });
    await expect(meetings.retryTranscription(9)).resolves.toEqual({ id: 10 });
    expect(apiRequestMock).toHaveBeenCalledWith("/recordings/9/transcription/retry", { method: "POST" });

    await meetings.downloadRecording(9, 11);
    expect(apiDownloadMock).toHaveBeenCalledWith("/recordings/9/files/11");
  });
});
