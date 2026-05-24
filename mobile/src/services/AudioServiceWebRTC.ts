/**
 * 基于 react-native-webrtc 的音频服务
 * 使用 Web Audio API，无需额外依赖
 */

import { Platform } from "react-native";

export type AudioType = "incoming_call" | "ringback";

// 生成简单的提示音（使用 Web Audio API 的振荡器）
class AudioServiceWebRTC {
  private static instance: AudioServiceWebRTC;
  private enabled: boolean = true;
  private audioContext: any = null;
  private oscillator: any = null;
  private gainNode: any = null;
  private playingAudio: AudioType | null = null;

  private constructor() {
    if (Platform.OS === "web") {
      // Web 平台使用 Web Audio API
      try {
        this.audioContext = new (window.AudioContext || (window as any).webkitAudioContext)();
      } catch {
        console.warn("[AudioService] Failed to create AudioContext");
      }
    }
  }

  public static getInstance(): AudioServiceWebRTC {
    if (!AudioServiceWebRTC.instance) {
      AudioServiceWebRTC.instance = new AudioServiceWebRTC();
    }
    return AudioServiceWebRTC.instance;
  }

  /**
   * 设置音频提醒开关
   */
  public setEnabled(enabled: boolean) {
    this.enabled = enabled;
    if (!enabled) {
      this.stopAll();
    }
  }

  /**
   * 获取当前音频提醒状态
   */
  public isEnabled(): boolean {
    return this.enabled;
  }

  /**
   * 播放音频（使用合成音调）
   */
  public async play(audioType: AudioType): Promise<void> {
    if (!this.enabled) {
      return;
    }

    this.playingAudio = audioType;

    try {
      if (Platform.OS === "web" && this.audioContext) {
        await this.playWebAudio(audioType);
      } else {
        // React Native 平台：播放系统提示音或使用原生实现
        // 这里可以集成 react-native-sound 或其他音频库
      }
    } catch (error) {
      console.error(`[AudioService] Error playing ${audioType}:`, error);
    }
  }

  /**
   * Web Audio API 实现
   */
  private async playWebAudio(audioType: AudioType) {
    if (!this.audioContext) {
      console.warn("[AudioService] No audio context available");
      return;
    }

    // 停止之前的音频
    this.stop(audioType);

    // 创建振荡器和增益节点
    this.oscillator = this.audioContext.createOscillator();
    this.gainNode = this.audioContext.createGain();

    // 连接节点
    this.oscillator.connect(this.gainNode);
    this.gainNode.connect(this.audioContext.destination);

    // 根据音频类型设置参数
    switch (audioType) {
      case "incoming_call":
        // 来电铃声：800Hz，每秒响0.5秒停0.5秒
        this.oscillator.frequency.value = 800;
        this.oscillator.type = "sine";
        this.gainNode.gain.value = 0.3;
        this.oscillator.start();

        // 间歇性播放
        setInterval(() => {
          if (this.gainNode && this.playingAudio === "incoming_call") {
            this.gainNode.gain.value = this.gainNode.gain.value > 0 ? 0 : 0.3;
          }
        }, 500);
        break;

      case "ringback":
        // 回铃音：450Hz，每秒响1秒停2秒
        this.oscillator.frequency.value = 450;
        this.oscillator.type = "sine";
        this.gainNode.gain.value = 0.2;
        this.oscillator.start();

        setInterval(() => {
          if (this.gainNode && this.playingAudio === "ringback") {
            this.gainNode.gain.value = this.gainNode.gain.value > 0 ? 0 : 0.2;
          }
        }, 1000);
        break;
    }
  }

  /**
   * 停止音频
   */
  public stop(audioType: AudioType): void {
    if (this.playingAudio === audioType) {
      this.playingAudio = null;

      if (this.oscillator) {
        try {
          this.oscillator.stop();
        } catch {
          // 忽略已经停止的振荡器错误
        }
        this.oscillator = null;
      }

      if (this.gainNode) {
        this.gainNode = null;
      }
    }
  }

  /**
   * 停止所有音频
   */
  public stopAll(): void {
    this.playingAudio = null;

    if (this.oscillator) {
      try {
        this.oscillator.stop();
      } catch {
        // 忽略错误
      }
      this.oscillator = null;
    }

    if (this.gainNode) {
      this.gainNode = null;
    }
  }

  /**
   * 释放资源
   */
  public dispose(): void {
    this.stopAll();
    if (this.audioContext && this.audioContext.close) {
      this.audioContext.close();
    }
  }
}

// 导出单例实例
export default AudioServiceWebRTC.getInstance();
