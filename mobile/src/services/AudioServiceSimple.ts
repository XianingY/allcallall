/**
 * Simplified audio service.
 *
 * Plays real notification sounds via expo-av (already a project dependency).
 * Wire up the actual sound assets by assigning modules in SOUND_ASSETS below;
 * until then playback stays in safe demo mode (logs only, never crashes).
 */

import { Audio } from 'expo-av';

export type AudioType = 'incoming_call' | 'ringback';

// Map each audio type to a bundled asset module. Drop the real sound files
// (e.g. under mobile/src/assets/audio/) and uncomment the requires to enable
// real playback:
//   incoming_call: require('../../assets/audio/incoming_call.wav'),
//   ringback: require('../../assets/audio/ringback.wav'),
const SOUND_ASSETS: Partial<Record<AudioType, any>> = {};

class AudioServiceSimple {
  private static instance: AudioServiceSimple;
  private enabled = true;
  private playingAudio: AudioType | null = null;
  private activeSound: Audio.Sound | null = null;

  private constructor() {}

  public static getInstance(): AudioServiceSimple {
    if (!AudioServiceSimple.instance) {
      AudioServiceSimple.instance = new AudioServiceSimple();
    }
    return AudioServiceSimple.instance;
  }

  /**
   * Toggle audio alerts on/off.
   */
  public setEnabled(enabled: boolean) {
    this.enabled = enabled;
    if (!enabled) {
      this.stopAll();
    }
  }

  /**
   * Current audio alert state.
   */
  public isEnabled(): boolean {
    return this.enabled;
  }

  /**
   * Play the given notification sound. Uses expo-av when an asset is wired in
   * SOUND_ASSETS; otherwise no-ops in demo mode. Failures are swallowed so the
   * caller never crashes on a missing/unsupported asset.
   */
  public async play(audioType: AudioType): Promise<void> {
    if (!this.enabled) {
      return;
    }

    this.playingAudio = audioType;

    const asset = SOUND_ASSETS[audioType];
    if (!asset) {
      return;
    }

    try {
      const { sound } = await Audio.Sound.createAsync(asset, {
        shouldPlay: true,
      });
      this.activeSound = sound;
    } catch (err) {
      console.warn('[AudioServiceSimple] playback failed', audioType, err);
    }
  }

  /**
   * Stop a specific sound.
   */
  public stop(audioType: AudioType): void {
    if (this.playingAudio === audioType) {
      this.playingAudio = null;
    }
    this.unloadActive();
  }

  /**
   * Stop all sounds.
   */
  public stopAll(): void {
    this.playingAudio = null;
    this.unloadActive();
  }

  /**
   * Release resources.
   */
  public dispose(): void {
    this.unloadActive();
  }

  private async unloadActive(): Promise<void> {
    if (this.activeSound) {
      try {
        await this.activeSound.unloadAsync();
      } catch (err) {
        console.warn('[AudioServiceSimple] unload failed', err);
      }
      this.activeSound = null;
    }
  }
}

// Export singleton instance.
export default AudioServiceSimple.getInstance();
