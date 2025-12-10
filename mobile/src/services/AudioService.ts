import Sound from "react-native-sound";

/**
 * 音频服务类
 * 负责播放来电铃声、拨号音、回铃音等
 */
export type AudioType = "incoming_call" | "ringback";

export class AudioService {
  private static instance: AudioService;
  private sounds: Map<AudioType, Sound> = new Map();
  private enabled: boolean = true;

  private constructor() {
    this.initializeSounds();
  }

  public static getInstance(): AudioService {
    if (!AudioService.instance) {
      AudioService.instance = new AudioService();
    }
    return AudioService.instance;
  }

  /**
   * 初始化音频文件
   * 使用系统原生支持的 WAV 格式
   */
  private initializeSounds() {
    try {
      // 注意：需要将音频文件放在 android/app/src/main/res/raw/ 目录下
      // 这里使用占位符，实际使用时需要添加音频文件

      // 来电铃声 - 需要创建文件: incoming_call.wav
      const incomingCallSound = new Sound(
        "incoming_call.wav",
        Sound.MAIN_BUNDLE,
        (error) => {
          if (error) {
            console.warn("Failed to load incoming_call.wav:", error);
          }
        }
      );
      this.sounds.set("incoming_call", incomingCallSound);

      // 回铃音 - 需要创建文件: ringback.wav
      const ringbackSound = new Sound(
        "ringback.wav",
        Sound.MAIN_BUNDLE,
        (error) => {
          if (error) {
            console.warn("Failed to load ringback.wav:", error);
          }
        }
      );
      this.sounds.set("ringback", ringbackSound);

      console.log("[AudioService] Sounds initialized");
    } catch (error) {
      console.error("[AudioService] Failed to initialize sounds:", error);
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
    return new Promise((resolve, reject) => {
      if (!this.enabled) {
        console.log("[AudioService] Audio is disabled, skipping play");
        resolve();
        return;
      }

      try {
        const sound = this.sounds.get(audioType);
        if (!sound) {
          console.warn(`[AudioService] Sound not found: ${audioType}`);
          resolve();
          return;
        }

        sound.setNumberOfLoops(-1); // 无限循环
        sound.play((success) => {
          if (success) {
            console.log(`[AudioService] Successfully played: ${audioType}`);
          } else {
            console.warn(`[AudioService] Failed to play: ${audioType}`);
          }
        });

        resolve();
      } catch (error) {
        console.error(`[AudioService] Error playing ${audioType}:`, error);
        reject(error);
      }
    });
  }

  /**
   * 停止音频
   */
  public stop(audioType: AudioType): void {
    try {
      const sound = this.sounds.get(audioType);
      if (sound) {
        sound.stop();
        console.log(`[AudioService] Stopped: ${audioType}`);
      }
    } catch (error) {
      console.error(`[AudioService] Error stopping ${audioType}:`, error);
    }
  }

  /**
   * 停止所有音频
   */
  public stopAll(): void {
    this.sounds.forEach((sound, type) => {
      try {
        sound.stop();
      } catch (error) {
        console.error(`[AudioService] Error stopping ${type}:`, error);
      }
    });
    console.log("[AudioService] Stopped all sounds");
  }

  /**
   * 释放资源
   */
  public dispose(): void {
    this.stopAll();
    this.sounds.forEach((sound) => {
      sound.release();
    });
    this.sounds.clear();
    console.log("[AudioService] Disposed");
  }
}

// 导出单例实例
export default AudioService.getInstance();
