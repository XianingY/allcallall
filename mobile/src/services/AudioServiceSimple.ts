/**
 * 简化版音频服务
 * 使用 Web Audio API (react-native-webrtc 内置)
 * 不依赖额外的第三方库
 */

import { Platform } from "react-native";

export type AudioType = "incoming_call" | "outgoing_dial" | "ringback";

// 模拟音频播放的简单实现
// 注意：此版本为演示用途，实际使用时需要真实的音频文件
class AudioServiceSimple {
  private static instance: AudioServiceSimple;
  private enabled: boolean = true;
  private playingAudio: AudioType | null = null;

  private constructor() {}

  public static getInstance(): AudioServiceSimple {
    if (!AudioServiceSimple.instance) {
      AudioServiceSimple.instance = new AudioServiceSimple();
    }
    return AudioServiceSimple.instance;
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
   * 注意：此版本只记录日志，实际音频播放需要真实的音频文件
   */
  public async play(audioType: AudioType): Promise<void> {
    if (!this.enabled) {
      console.log("[AudioService] Audio is disabled, skipping play");
      return;
    }

    console.log(`[AudioService] Playing: ${audioType}`);
    this.playingAudio = audioType;

    // TODO: 实现真实的音频播放
    // 方案1: 使用 react-native-sound
    // 方案2: 使用 react-native-audio-recorder-player
    // 方案3: 使用 HTML5 Audio (react-native-webrtc 已支持)

    // 这里可以添加真实的音频播放逻辑
    // 例如：
    // const sound = new Sound('incoming_call.wav', Sound.MAIN_BUNDLE, ...);
    // sound.play();
  }

  /**
   * 停止音频
   */
  public stop(audioType: AudioType): void {
    console.log(`[AudioService] Stopping: ${audioType}`);
    if (this.playingAudio === audioType) {
      this.playingAudio = null;
    }
  }

  /**
   * 停止所有音频
   */
  public stopAll(): void {
    console.log("[AudioService] Stopping all audio");
    this.playingAudio = null;
  }

  /**
   * 释放资源
   */
  public dispose(): void {
    this.stopAll();
    console.log("[AudioService] Disposed");
  }
}

// 导出单例实例
export default AudioServiceSimple.getInstance();
