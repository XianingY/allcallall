/**
 * 基于 expo-av 的音频服务 - 增强版
 * 支持真实音频文件播放
 *
 * 功能特性：
 * - 真实音频文件支持
 * - 音频预加载
 * - 循环播放
 * - 音量控制
 * - 后台播放
 */

import { Audio } from "expo-av";
import { Platform } from "react-native";

export type AudioType = "incoming_call" | "ringback";

interface AudioFile {
  type: AudioType;
  source: any; // require() 或 URI 字符串
  name: string;
}

class AudioServiceExpo {
  private static instance: AudioServiceExpo;
  private enabled: boolean = true;
  private soundObjects: Map<AudioType, Audio.Sound> = new Map();
  private initialized: boolean = false;
  private loading: boolean = false;

  // 音频文件配置 - 支持MP3格式，推荐使用MP3以减小文件大小
  private readonly audioFiles: AudioFile[] = [
    {
      type: "incoming_call",
      source: require("../assets/sounds/incoming_call.mp3"),
      name: "incoming_call.mp3"
    },
    {
      type: "ringback",
      source: require("../assets/sounds/ringback.mp3"),
      name: "ringback.mp3"
    }
  ];

  private constructor() {
    this.initializeAudio();
  }

  public static getInstance(): AudioServiceExpo {
    if (!AudioServiceExpo.instance) {
      AudioServiceExpo.instance = new AudioServiceExpo();
    }
    return AudioServiceExpo.instance;
  }

  /**
   * 初始化音频系统
   */
  private async initializeAudio() {
    try {
      // 设置音频模式
      await Audio.setAudioModeAsync({
        staysActiveInBackground: true,
        playsInSilentModeIOS: true,
        shouldDuckAndroid: true,
        playThroughEarpieceAndroid: false
      });
      console.log("[AudioService] Audio mode set");
      this.initialized = true;

      // 预加载音频文件
      await this.preloadAudioFiles();
    } catch (error) {
      console.warn("[AudioService] Failed to initialize audio:", error);
    }
  }

  /**
   * 预加载所有音频文件
   */
  private async preloadAudioFiles(): Promise<void> {
    if (this.loading) {
      console.log("[AudioService] Already loading audio files");
      return;
    }

    this.loading = true;
    console.log("[AudioService] Preloading audio files...");

    try {
      const loadPromises = this.audioFiles.map(async (audioFile) => {
        try {
          const sound = new Audio.Sound();

          // 加载音频文件
          await sound.loadAsync(audioFile.source, {
            shouldPlay: false,
            isLooping: audioFile.type === "incoming_call" || audioFile.type === "ringback"
          });

          // 设置音量
          await sound.setVolumeAsync(0.8);

          this.soundObjects.set(audioFile.type, sound);
          console.log(`[AudioService] ✓ Loaded: ${audioFile.name}`);
        } catch (error) {
          console.error(`[AudioService] Failed to load ${audioFile.name}:`, error);
          // 如果加载失败，创建一个空的Sound对象作为占位符
          const emptySound = new Audio.Sound();
          this.soundObjects.set(audioFile.type, emptySound);
        }
      });

      await Promise.all(loadPromises);
      console.log(`[AudioService] ✓ All audio files loaded (${this.soundObjects.size} files)`);
    } catch (error) {
      console.error("[AudioService] Error preloading audio files:", error);
    } finally {
      this.loading = false;
    }
  }

  /**
   * 重新加载音频文件
   */
  public async reloadAudioFiles(): Promise<void> {
    console.log("[AudioService] Reloading audio files...");
    await this.unloadAudioFiles();
    await this.preloadAudioFiles();
  }

  /**
   * 卸载所有音频文件
   */
  private async unloadAudioFiles(): Promise<void> {
    console.log("[AudioService] Unloading audio files...");

    try {
      const unloadPromises = Array.from(this.soundObjects.values()).map(
        async (sound) => {
          try {
            await sound.unloadAsync();
          } catch (error) {
            console.warn("[AudioService] Error unloading sound:", error);
          }
        }
      );

      await Promise.all(unloadPromises);
      this.soundObjects.clear();
      console.log("[AudioService] ✓ All audio files unloaded");
    } catch (error) {
      console.error("[AudioService] Error unloading audio files:", error);
    }
  }

  /**
   * 设置音频提醒开关
   */
  public setEnabled(enabled: boolean) {
    this.enabled = enabled;
    if (!enabled) {
      this.stopAll();
    }
    console.log("[AudioService] Audio enabled:", enabled);
  }

  /**
   * 获取当前音频提醒状态
   */
  public isEnabled(): boolean {
    return this.enabled;
  }

  /**
   * 播放音频
   */
  public async play(audioType: AudioType): Promise<void> {
    if (!this.enabled) {
      console.log("[AudioService] Audio is disabled, skipping play");
      return;
    }

    console.log(`[AudioService] Playing: ${audioType}`);

    try {
      if (!this.initialized) {
        await this.initializeAudio();
      }

      // 确保音频已加载
      if (!this.soundObjects.has(audioType)) {
        console.warn(`[AudioService] Audio not loaded: ${audioType}, attempting to reload...`);
        await this.preloadAudioFiles();
      }

      // 停止之前的音频
      this.stopAll();

      const sound = this.soundObjects.get(audioType);
      if (sound) {
        // 设置循环播放（来电和回铃音需要循环）
        if (audioType === "incoming_call" || audioType === "ringback") {
          await sound.setIsLoopingAsync(true);
        }

        // 播放音频
        await sound.setPositionAsync(0); // 从头开始播放
        await sound.playAsync();

        console.log(`[AudioService] ✓ Playing: ${audioType}`);
      } else {
        console.warn(`[AudioService] No sound object for: ${audioType}`);
      }
    } catch (error) {
      console.error(`[AudioService] Error playing ${audioType}:`, error);
    }
  }

  /**
   * 停止音频
   */
  public async stop(audioType: AudioType): Promise<void> {
    console.log(`[AudioService] Stopping: ${audioType}`);

    try {
      const sound = this.soundObjects.get(audioType);
      if (sound) {
        // 停止并重置位置
        await sound.stopAsync();
        await sound.setPositionAsync(0);
        await sound.setIsLoopingAsync(false);

        console.log(`[AudioService] ✓ Stopped: ${audioType}`);
      }
    } catch (error) {
      console.error(`[AudioService] Error stopping ${audioType}:`, error);
    }
  }

  /**
   * 暂停音频
   */
  public async pause(audioType: AudioType): Promise<void> {
    console.log(`[AudioService] Pausing: ${audioType}`);

    try {
      const sound = this.soundObjects.get(audioType);
      if (sound) {
        await sound.pauseAsync();
        console.log(`[AudioService] ✓ Paused: ${audioType}`);
      }
    } catch (error) {
      console.error(`[AudioService] Error pausing ${audioType}:`, error);
    }
  }

  /**
   * 恢复音频播放
   */
  public async resume(audioType: AudioType): Promise<void> {
    console.log(`[AudioService] Resuming: ${audioType}`);

    try {
      const sound = this.soundObjects.get(audioType);
      if (sound) {
        await sound.playAsync();
        console.log(`[AudioService] ✓ Resumed: ${audioType}`);
      }
    } catch (error) {
      console.error(`[AudioService] Error resuming ${audioType}:`, error);
    }
  }

  /**
   * 设置音量 (0.0 - 1.0)
   */
  public async setVolume(audioType: AudioType, volume: number): Promise<void> {
    try {
      const sound = this.soundObjects.get(audioType);
      if (sound) {
        await sound.setVolumeAsync(volume);
        console.log(`[AudioService] Volume set for ${audioType}: ${volume}`);
      }
    } catch (error) {
      console.error(`[AudioService] Error setting volume for ${audioType}:`, error);
    }
  }

  /**
   * 获取当前播放状态
   */
  public async getStatus(audioType: AudioType): Promise<Audio.SoundStatus | null> {
    try {
      const sound = this.soundObjects.get(audioType);
      if (sound) {
        const status = await sound.getStatusAsync();
        return status;
      }
      return null;
    } catch (error) {
      console.error(`[AudioService] Error getting status for ${audioType}:`, error);
      return null;
    }
  }

  /**
   * 停止所有音频
   */
  public async stopAll(): Promise<void> {
    console.log("[AudioService] Stopping all audio");

    try {
      const stopPromises = Array.from(this.soundObjects.entries()).map(
        async ([type, sound]) => {
          try {
            await sound.stopAsync();
            await sound.setPositionAsync(0);
            await sound.setIsLoopingAsync(false);
            console.log(`[AudioService] ✓ Stopped: ${type}`);
          } catch (error) {
            console.warn(`[AudioService] Error stopping ${type}:`, error);
          }
        }
      );

      await Promise.all(stopPromises);
    } catch (error) {
      console.error("[AudioService] Error stopping all audio:", error);
    }
  }

  /**
   * 检查音频文件是否存在
   */
  public checkAudioFiles(): { [key: string]: boolean } {
    const result: { [key: string]: boolean } = {};

    this.audioFiles.forEach((audioFile) => {
      result[audioFile.name] = this.soundObjects.has(audioFile.type);
    });

    return result;
  }

  /**
   * 释放资源
   */
  public async dispose(): Promise<void> {
    console.log("[AudioService] Disposing...");

    try {
      await this.stopAll();
      await this.unloadAudioFiles();
      this.initialized = false;
    } catch (error) {
      console.error("[AudioService] Error disposing:", error);
    }
  }
}

// 导出单例实例
export default AudioServiceExpo.getInstance();
