/**
 * 震动反馈服务
 * 提供来电、拨号等场景的震动模式
 */

import { Vibration } from "react-native";

export type VibrationType = "incoming_call" | "outgoing_dial" | "ringback" | "call_connected" | "call_ended";

interface VibrationPattern {
  [key: string]: number[];
}

class VibrationService {
  private static instance: VibrationService;
  private enabled: boolean = true;
  private isVibrating: boolean = false;

  // 震动模式定义
  private readonly patterns: VibrationPattern = {
    // 来电：长震-短停-长震循环 (持续震动)
    incoming_call: [0, 500, 250, 500],

    // 拨号：短促震动一次
    outgoing_dial: [0, 200],

    // 回铃：中震动-短停-中震动循环
    ringback: [0, 1000, 500],

    // 通话接通：短促双震动
    call_connected: [0, 150, 100, 150],

    // 通话结束：单次长震动
    call_ended: [0, 400]
  };

  private constructor() {
    console.log("[VibrationService] Initialized");
  }

  public static getInstance(): VibrationService {
    if (!VibrationService.instance) {
      VibrationService.instance = new VibrationService();
    }
    return VibrationService.instance;
  }

  /**
   * 设置震动开关
   */
  public setEnabled(enabled: boolean): void {
    this.enabled = enabled;
    if (!enabled) {
      this.cancel();
    }
    console.log("[VibrationService] Vibration enabled:", enabled);
  }

  /**
   * 获取当前震动状态
   */
  public isEnabled(): boolean {
    return this.enabled;
  }

  /**
   * 震动
   */
  public vibrate(type: VibrationType): void {
    if (!this.enabled) {
      console.log("[VibrationService] Vibration is disabled, skipping");
      return;
    }

    console.log(`[VibrationService] Vibrating: ${type}`);

    try {
      const pattern = this.patterns[type];

      if (!pattern) {
        console.warn(`[VibrationService] No pattern defined for: ${type}`);
        return;
      }

      // 对于需要循环的震动（来电、回铃），使用循环模式
      if (type === "incoming_call" || type === "ringback") {
        Vibration.vibrate(pattern, true);
        this.isVibrating = true;
      } else {
        // 单次震动
        Vibration.vibrate(pattern);
        this.isVibrating = false;
      }
    } catch (error) {
      console.error(`[VibrationService] Error vibrating ${type}:`, error);
    }
  }

  /**
   * 取消震动
   */
  public cancel(): void {
    if (this.isVibrating) {
      console.log("[VibrationService] Canceling vibration");
      Vibration.cancel();
      this.isVibrating = false;
    }
  }

  /**
   * 自定义震动模式
   */
  public vibrateCustom(pattern: number[], repeat: boolean = false): void {
    if (!this.enabled) {
      console.log("[VibrationService] Vibration is disabled, skipping custom pattern");
      return;
    }

    console.log("[VibrationService] Vibrating with custom pattern");
    Vibration.vibrate(pattern, repeat);
    this.isVibrating = repeat;
  }

  /**
   * 获取震动强度建议
   */
  public getVibrationIntensity(type: VibrationType): "light" | "medium" | "heavy" {
    switch (type) {
      case "incoming_call":
        return "medium";
      case "outgoing_dial":
        return "light";
      case "ringback":
        return "medium";
      case "call_connected":
        return "light";
      case "call_ended":
        return "heavy";
      default:
        return "medium";
    }
  }

  /**
   * 清理资源
   */
  public dispose(): void {
    console.log("[VibrationService] Disposing...");
    this.cancel();
  }
}

// 导出单例实例
export default VibrationService.getInstance();
