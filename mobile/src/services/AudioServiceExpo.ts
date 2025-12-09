/**
 * 基于 expo-av 的音频服务
 * 使用 Expo 内置音频功能，无需额外配置
 */

import { Audio } from "expo-av";
import { Platform } from "react-native";

export type AudioType = "incoming_call" | "outgoing_dial" | "ringback";

class AudioServiceExpo {
  private static instance: AudioServiceExpo;
  private enabled: boolean = true;
  private soundObjects: Map<AudioType, Audio.Sound> = new Map();
  private initialized: boolean = false;

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
    } catch (error) {
      console.warn("[AudioService] Failed to initialize audio:", error);
    }
  }

  /**
   * 创建音频对象（使用合成音调）
   */
  private async createSyntheticSound(audioType: AudioType): Promise<Audio.Sound> {
    // 注意：这里使用合成音调，实际项目中应该加载真实的音频文件
    // 使用 expo-av 的 AVPlaybackSourceObject

    // 临时使用系统提示音替代
    // 真实实现时，应该从本地文件或网络加载音频
    const soundObject = new Audio.Sound();

    try {
      // 这里可以加载真实的音频文件
      // await soundObject.loadAsync(require('./sounds/incoming_call.wav'));
      // 或从网络加载
      // await soundObject.loadAsync({ uri: 'https://example.com/sounds/incoming_call.wav' });

      console.log(`[AudioService] Created synthetic sound for: ${audioType}`);
      return soundObject;
    } catch (error) {
      console.error(`[AudioService] Failed to create sound for ${audioType}:`, error);
      throw error;
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

      // 停止之前的音频
      this.stopAll();

      // 创建或获取音频对象
      let sound = this.soundObjects.get(audioType);
      if (!sound) {
        sound = await this.createSyntheticSound(audioType);
        this.soundObjects.set(audioType, sound);
      }

      // 播放音频
      // 注意：当前实现使用合成音调，实际使用时请加载真实音频文件
      console.log(`[AudioService] ${audioType} would play (synthetic mode)`);

      // 如果有真实音频文件，使用以下代码：
      // await sound.setIsLoopingAsync(true);
      // await sound.playAsync();

      // 合成音调实现（临时方案）
      this.playSyntheticTone(audioType);

    } catch (error) {
      console.error(`[AudioService] Error playing ${audioType}:`, error);
    }
  }

  /**
   * 播放合成音调（临时实现）
   */
  private playSyntheticTone(audioType: AudioType) {
    // 使用 Web API 或原生实现生成提示音
    // 这里仅记录日志，实际项目需要集成真实的音频播放

    switch (audioType) {
      case "incoming_call":
        console.log("🔔 Ringing... (synthetic)");
        break;
      case "outgoing_dial":
        console.log("📞 Dialing... (synthetic)");
        break;
      case "ringback":
        console.log("🔄 Ringback... (synthetic)");
        break;
    }

    // TODO: 实现真实的音频播放
    // 可以使用 react-native-sound 或 expo-av 的音频文件播放
  }

  /**
   * 停止音频
   */
  public async stop(audioType: AudioType): Promise<void> {
    console.log(`[AudioService] Stopping: ${audioType}`);

    try {
      const sound = this.soundObjects.get(audioType);
      if (sound) {
        // 真实音频文件停止
        // await sound.stopAsync();
        // await sound.unloadAsync();

        // 合成音调停止
        console.log(`[AudioService] Stopped: ${audioType}`);
      }
    } catch (error) {
      console.error(`[AudioService] Error stopping ${audioType}:`, error);
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
            // await sound.stopAsync();
            console.log(`[AudioService] Stopped: ${type}`);
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
   * 释放资源
   */
  public async dispose(): Promise<void> {
    console.log("[AudioService] Disposing...");

    try {
      await this.stopAll();

      const unloadPromises = Array.from(this.soundObjects.values()).map(
        async (sound) => {
          try {
            // await sound.unloadAsync();
          } catch (error) {
            console.warn("[AudioService] Error unloading sound:", error);
          }
        }
      );

      await Promise.all(unloadPromises);
      this.soundObjects.clear();
    } catch (error) {
      console.error("[AudioService] Error disposing:", error);
    }
  }
}

// 导出单例实例
export default AudioServiceExpo.getInstance();
