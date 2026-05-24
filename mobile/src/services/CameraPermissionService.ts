/**
 * 摄像头权限服务
 * 处理摄像头和麦克风权限请求和检查
 */
import permissionsAdapter, { type PermissionResult } from "../platform/permissionsAdapter";

class CameraPermissionService {
  /**
   * 检查并请求所有必要的权限
   */
  async checkPermissions(): Promise<PermissionResult> {
    const result = await permissionsAdapter.requestMeetingPermissions();
    if (!result.allGranted) {
      console.warn("[CameraPermissionService] Camera or microphone permission denied");
    }
    return result;
  }

  /**
   * 检查摄像头权限（仅检查，不请求）
   */
  async hasCameraPermission(): Promise<boolean> {
    return permissionsAdapter.hasCameraPermission();
  }

  /**
   * 检查麦克风权限（仅检查，不请求）
   */
  async hasMicrophonePermission(): Promise<boolean> {
    return permissionsAdapter.hasMicrophonePermission();
  }

  /**
   * 显示权限被拒绝的提示
   */
  showPermissionDeniedAlert(missingPermissions: string[]): void {
    permissionsAdapter.showPermissionDeniedAlert(missingPermissions);
  }

  /**
   * 根据权限结果显示相应的提示
   */
  handlePermissionResult(result: PermissionResult): void {
    if (result.allGranted) {
      return;
    }

    const missingPermissions: string[] = [];
    
    if (!result.camera) {
      missingPermissions.push("摄像头 / Camera");
    }
    
    if (!result.microphone) {
      missingPermissions.push("麦克风 / Microphone");
    }

    if (missingPermissions.length > 0) {
      this.showPermissionDeniedAlert(missingPermissions);
    }
  }
}

// 导出单例实例
export default new CameraPermissionService();
