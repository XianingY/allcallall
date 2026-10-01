/**
 * Notification-audio service built on the maintained `expo-audio` API.
 *
 * The service owns two preloaded alert players. Callers only need `play`,
 * `stopAll`, and speakerphone routing; keeping the lower-level player state
 * private prevents signalling contexts from leaking native audio resources.
 */

import {
  createAudioPlayer,
  setAudioModeAsync,
  type AudioPlayer,
  type AudioSource,
  type AudioStatus,
} from "expo-audio";

export type AudioType = "incoming_call" | "ringback";

interface AudioFile {
  type: AudioType;
  source: AudioSource;
  name: string;
}

class AudioServiceExpo {
  private static instance: AudioServiceExpo;
  private enabled = true;
  private players = new Map<AudioType, AudioPlayer>();
  private initialized = false;
  private loading = false;
  private isSpeakerOn = false;

  private readonly audioFiles: AudioFile[] = [
    {
      type: "incoming_call",
      source: require("../assets/sounds/incoming_call.mp3"),
      name: "incoming_call.mp3",
    },
    {
      type: "ringback",
      source: require("../assets/sounds/ringback.mp3"),
      name: "ringback.mp3",
    },
  ];

  private constructor() {
    void this.initializeAudio();
  }

  public static getInstance(): AudioServiceExpo {
    if (!AudioServiceExpo.instance) {
      AudioServiceExpo.instance = new AudioServiceExpo();
    }
    return AudioServiceExpo.instance;
  }

  private async configureAudioMode(shouldRouteThroughEarpiece: boolean): Promise<void> {
    await setAudioModeAsync({
      interruptionMode: "duckOthers",
      playsInSilentMode: true,
      shouldPlayInBackground: true,
      shouldRouteThroughEarpiece,
    });
  }

  private async initializeAudio(): Promise<void> {
    try {
      await this.configureAudioMode(false);
      this.initialized = true;
      await this.preloadAudioFiles();
    } catch (error) {
      console.warn("[AudioService] Failed to initialize audio:", error);
    }
  }

  private async preloadAudioFiles(): Promise<void> {
    if (this.loading) {
      return;
    }

    this.loading = true;
    try {
      for (const audioFile of this.audioFiles) {
        if (this.players.has(audioFile.type)) {
          continue;
        }

        try {
          const player = createAudioPlayer(audioFile.source);
          player.loop = true;
          player.volume = 0.8;
          this.players.set(audioFile.type, player);
        } catch (error) {
          console.error(`[AudioService] Failed to load ${audioFile.name}:`, error);
        }
      }
    } finally {
      this.loading = false;
    }
  }

  public async reloadAudioFiles(): Promise<void> {
    await this.unloadAudioFiles();
    await this.preloadAudioFiles();
  }

  private async unloadAudioFiles(): Promise<void> {
    for (const [type, player] of this.players) {
      try {
        player.remove();
      } catch (error) {
        console.warn(`[AudioService] Error unloading ${type}:`, error);
      }
    }
    this.players.clear();
  }

  public setEnabled(enabled: boolean) {
    this.enabled = enabled;
    if (!enabled) {
      void this.stopAll();
    }
  }

  public isEnabled(): boolean {
    return this.enabled;
  }

  public async play(audioType: AudioType): Promise<void> {
    if (!this.enabled) {
      return;
    }

    try {
      if (!this.initialized) {
        await this.initializeAudio();
      }
      if (!this.players.has(audioType)) {
        console.warn(
          `[AudioService] Audio not loaded: ${audioType}, attempting to reload...`,
        );
        await this.preloadAudioFiles();
      }

      // Alert types overlap in time, so make switching alerts deterministic:
      // finish stopping the previous one before starting the requested one.
      await this.stopAll();

      const player = this.players.get(audioType);
      if (!player) {
        console.warn(`[AudioService] No audio player for: ${audioType}`);
        return;
      }

      player.loop = true;
      await player.seekTo(0);
      player.play();
    } catch (error) {
      console.error(`[AudioService] Error playing ${audioType}:`, error);
    }
  }

  public async stop(audioType: AudioType): Promise<void> {
    const player = this.players.get(audioType);
    if (!player) {
      return;
    }

    try {
      player.pause();
      await player.seekTo(0);
      player.loop = false;
    } catch (error) {
      console.error(`[AudioService] Error stopping ${audioType}:`, error);
    }
  }

  public async pause(audioType: AudioType): Promise<void> {
    const player = this.players.get(audioType);
    if (!player) {
      return;
    }

    try {
      player.pause();
    } catch (error) {
      console.error(`[AudioService] Error pausing ${audioType}:`, error);
    }
  }

  public async resume(audioType: AudioType): Promise<void> {
    const player = this.players.get(audioType);
    if (!player) {
      return;
    }

    try {
      player.play();
    } catch (error) {
      console.error(`[AudioService] Error resuming ${audioType}:`, error);
    }
  }

  public async setVolume(audioType: AudioType, volume: number): Promise<void> {
    const player = this.players.get(audioType);
    if (!player) {
      return;
    }

    try {
      player.volume = volume;
    } catch (error) {
      console.error(`[AudioService] Error setting volume for ${audioType}:`, error);
    }
  }

  public async getStatus(audioType: AudioType): Promise<AudioStatus | null> {
    const player = this.players.get(audioType);
    return player?.currentStatus ?? null;
  }

  public async stopAll(): Promise<void> {
    try {
      await Promise.all(
        Array.from(this.players.entries()).map(async ([type, player]) => {
          try {
            player.pause();
            await player.seekTo(0);
            player.loop = false;
          } catch (error) {
            console.warn(`[AudioService] Error stopping ${type}:`, error);
          }
        }),
      );
    } catch (error) {
      console.error("[AudioService] Error stopping all audio:", error);
    }
  }

  public checkAudioFiles(): Record<string, boolean> {
    return Object.fromEntries(
      this.audioFiles.map((audioFile) => [
        audioFile.name,
        this.players.has(audioFile.type),
      ]),
    );
  }

  public async setSpeakerphone(on: boolean): Promise<void> {
    try {
      this.isSpeakerOn = on;
      await this.configureAudioMode(!on);
    } catch (error) {
      console.error("[AudioService] Failed to set speakerphone:", error);
    }
  }

  public getSpeakerphone(): boolean {
    return this.isSpeakerOn;
  }

  public async dispose(): Promise<void> {
    try {
      await this.stopAll();
      await this.unloadAudioFiles();
      this.initialized = false;
    } catch (error) {
      console.error("[AudioService] Error disposing:", error);
    }
  }
}

export default AudioServiceExpo.getInstance();
