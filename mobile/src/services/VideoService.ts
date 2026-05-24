/**
 * 视频服务
 * 负责视频流管理、摄像头控制、视频质量管理等功能
 */

import { mediaDevices as webrtcMediaDevices, MediaStream } from "../platform/rtc";

export type CameraFacing = "front" | "back";
export type VideoQuality = "low" | "medium" | "high";

interface VideoConstraints {
  width: number;
  height: number;
  frameRate: number;
}

// 视频质量配置
const VIDEO_QUALITY_PRESETS: Record<VideoQuality, VideoConstraints> = {
  low: {
    width: 320,
    height: 240,
    frameRate: 15
  },
  medium: {
    width: 640,
    height: 480,
    frameRate: 24
  },
  high: {
    width: 1280,
    height: 720,
    frameRate: 30
  }
};

class VideoService {
  private currentStream: MediaStream | null = null;
  private currentFacingMode: CameraFacing = "front";
  private currentQuality: VideoQuality = "medium";
  private isInitialized: boolean = false;

  /**
   * 初始化视频服务
   */
  async initialize(): Promise<void> {
    if (this.isInitialized) {
      return;
    }

    
    if (!webrtcMediaDevices) {
      throw new Error("WebRTC mediaDevices not available");
    }

    this.isInitialized = true;
  }

  /**
   * 获取本地媒体流（音频+视频）
   * @param audioEnabled 是否启用音频
   * @param videoEnabled 是否启用视频
   * @param facingMode 摄像头方向
   * @param quality 视频质量
   */
  async getLocalStream(
    audioEnabled: boolean = true,
    videoEnabled: boolean = false,
    facingMode: CameraFacing = "front",
    quality: VideoQuality = "medium"
  ): Promise<MediaStream | null> {
    try {

      // 停止旧的流
      if (this.currentStream) {
        this.stopStream(this.currentStream);
      }

      const constraints: any = {
        audio: audioEnabled,
        video: videoEnabled ? this.getVideoConstraints(facingMode, quality) : false
      };


      const stream = await webrtcMediaDevices.getUserMedia(constraints);
      
      this.currentStream = stream;
      this.currentFacingMode = facingMode;
      this.currentQuality = quality;


      return stream;
    } catch (error) {
      console.error("[VideoService] Failed to get local stream:", error);
      throw error;
    }
  }

  /**
   * 获取视频约束配置
   */
  private getVideoConstraints(facingMode: CameraFacing, quality: VideoQuality): any {
    const preset = VIDEO_QUALITY_PRESETS[quality];
    
    return {
      width: { ideal: preset.width },
      height: { ideal: preset.height },
      frameRate: { ideal: preset.frameRate },
      facingMode: facingMode === "front" ? "user" : "environment"
    };
  }

  /**
   * 切换摄像头（前置/后置）
   */
  async switchCamera(): Promise<MediaStream | null> {
    try {
      const newFacingMode: CameraFacing = this.currentFacingMode === "front" ? "back" : "front";

      // 检查当前流中是否有视频轨道
      const hasVideo = (this.currentStream?.getVideoTracks().length ?? 0) > 0;
      const hasAudio = (this.currentStream?.getAudioTracks().length ?? 0) > 0;

      if (!hasVideo) {
        console.warn("[VideoService] No video track found, cannot switch camera");
        return this.currentStream;
      }

      // 获取新的流
      const newStream = await this.getLocalStream(
        hasAudio,
        true,
        newFacingMode,
        this.currentQuality
      );

      return newStream;
    } catch (error) {
      console.error("[VideoService] Failed to switch camera:", error);
      throw error;
    }
  }

  /**
   * 启用/禁用视频轨道
   */
  toggleVideoTrack(enabled: boolean): void {
    if (!this.currentStream) {
      console.warn("[VideoService] No current stream to toggle video");
      return;
    }

    const videoTracks = this.currentStream.getVideoTracks();
    videoTracks.forEach((track) => {
      track.enabled = enabled;
    });
  }

  /**
   * 启用/禁用音频轨道
   */
  toggleAudioTrack(enabled: boolean): void {
    if (!this.currentStream) {
      console.warn("[VideoService] No current stream to toggle audio");
      return;
    }

    const audioTracks = this.currentStream.getAudioTracks();
    audioTracks.forEach((track) => {
      track.enabled = enabled;
    });
  }

  /**
   * 停止媒体流
   */
  stopStream(stream: MediaStream): void {
    try {
      stream.getTracks().forEach((track) => {
        track.stop();
      });
    } catch (error) {
      console.error("[VideoService] Error stopping stream:", error);
    }
  }

  /**
   * 停止当前流
   */
  stopCurrentStream(): void {
    if (this.currentStream) {
      this.stopStream(this.currentStream);
      this.currentStream = null;
    }
  }

  /**
   * 获取当前摄像头方向
   */
  getCurrentFacingMode(): CameraFacing {
    return this.currentFacingMode;
  }

  /**
   * 获取当前视频质量
   */
  getCurrentQuality(): VideoQuality {
    return this.currentQuality;
  }

  /**
   * 设置视频质量
   */
  async setVideoQuality(quality: VideoQuality): Promise<MediaStream | null> {
    try {

      const hasVideo = (this.currentStream?.getVideoTracks().length ?? 0) > 0;
      const hasAudio = (this.currentStream?.getAudioTracks().length ?? 0) > 0;

      if (!hasVideo) {
        console.warn("[VideoService] No video track found, cannot change quality");
        return this.currentStream;
      }

      // 获取新的流
      const newStream = await this.getLocalStream(
        hasAudio,
        true,
        this.currentFacingMode,
        quality
      );

      return newStream;
    } catch (error) {
      console.error("[VideoService] Failed to set video quality:", error);
      throw error;
    }
  }

  /**
   * 检查是否有活动的视频轨道
   */
  hasActiveVideoTrack(): boolean {
    if (!this.currentStream) return false;
    
    const videoTracks = this.currentStream.getVideoTracks();
    return videoTracks.some(track => track.enabled && track.readyState === "live");
  }

  /**
   * 检查是否有活动的音频轨道
   */
  hasActiveAudioTrack(): boolean {
    if (!this.currentStream) return false;
    
    const audioTracks = this.currentStream.getAudioTracks();
    return audioTracks.some(track => track.enabled && track.readyState === "live");
  }

  /**
   * 清理资源
   */
  cleanup(): void {
    this.stopCurrentStream();
    this.isInitialized = false;
  }
}

// 导出单例实例
export default new VideoService();
