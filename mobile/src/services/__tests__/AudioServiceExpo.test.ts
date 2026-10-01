import {
  createAudioPlayer,
  setAudioModeAsync,
} from "expo-audio";

import AudioService from "../AudioServiceExpo";

jest.mock("expo-audio", () => ({
  createAudioPlayer: jest.fn(() => ({
    loop: false,
    pause: jest.fn(),
    play: jest.fn(),
    remove: jest.fn(),
    seekTo: jest.fn().mockResolvedValue(undefined),
    volume: 1,
  })),
  setAudioModeAsync: jest.fn().mockResolvedValue(undefined),
}));

const createAudioPlayerMock =
  createAudioPlayer as jest.MockedFunction<typeof createAudioPlayer>;
const setAudioModeAsyncMock =
  setAudioModeAsync as jest.MockedFunction<typeof setAudioModeAsync>;

type MockPlayer = ReturnType<typeof createAudioPlayer> & {
  loop: boolean;
  pause: jest.Mock;
  play: jest.Mock;
  seekTo: jest.Mock;
  volume: number;
};

async function waitForInitialization() {
  await Promise.resolve();
  await Promise.resolve();
}

const players: MockPlayer[] = [];

describe("AudioServiceExpo", () => {
  beforeAll(async () => {
    await waitForInitialization();
    players.push(
      ...createAudioPlayerMock.mock.results.map(
        (result) => result.value as MockPlayer,
      ),
    );
  });

  beforeEach(() => {
    players.forEach((player) => {
      player.pause.mockClear();
      player.play.mockClear();
      player.seekTo.mockClear();
    });
  });

  it("configures the ducked background mode and preloads notification players", async () => {
    await waitForInitialization();

    expect(setAudioModeAsyncMock).toHaveBeenCalledWith(
      expect.objectContaining({
        interruptionMode: "duckOthers",
        playsInSilentMode: true,
        shouldPlayInBackground: true,
      }),
    );
    expect(createAudioPlayerMock).toHaveBeenCalledTimes(2);
    expect(createAudioPlayerMock).toHaveBeenCalledWith(expect.anything());
  });

  it("plays the requested alert from the beginning and stops other alerts", async () => {
    await waitForInitialization();
    await AudioService.play("incoming_call");

    const [incomingCall, ringback] = players;

    expect(incomingCall.seekTo).toHaveBeenCalledWith(0);
    expect(incomingCall.play).toHaveBeenCalledTimes(1);
    expect(ringback.pause).toHaveBeenCalled();
    expect(ringback.seekTo).toHaveBeenCalledWith(0);
  });

  it("keeps alert playback disabled until explicitly enabled", async () => {
    await waitForInitialization();
    AudioService.setEnabled(false);
    await AudioService.play("ringback");

    expect(players.every((player) => player.play.mock.calls.length === 0)).toBe(true);

    AudioService.setEnabled(true);
    await AudioService.play("ringback");
    expect(players[1].play).toHaveBeenCalledTimes(1);
  });
});
