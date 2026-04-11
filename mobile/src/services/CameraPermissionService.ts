/**
 * 摄像头权限服务
 * 处理摄像头和麦克风权限请求和检查
 */

import { Platform, PermissionsAndroid, Alert } from "react-native";

export interface PermissionResult {
  camera: boolean;
  microphone: boolean;
  allGranted: boolean;
}

class CameraPermissionService {
  /**
   * 检查并请求所有必要的权限
   */
  async checkPermissions(): Promise<PermissionResult> {

    if (Platform.OS === "android") {
      return await this.checkAndroidPermissions();
    } else if (Platform.OS === "ios") {
      // iOS 权限在首次使用时自动请求
      return {
        camera: true,
        microphone: true,
        allGranted: true
      };
    }

    return {
      camera: false,
      microphone: false,
      allGranted: false
    };
  }

  /**
   * 检查 Android 权限
   */
  private async checkAndroidPermissions(): Promise<PermissionResult> {
    try {
      const permissions: string[] = [
        PermissionsAndroid.PERMISSIONS.CAMERA,
        PermissionsAndroid.PERMISSIONS.RECORD_AUDIO
      ];

      // Android 12+ 需要蓝牙连接权限（用于蓝牙耳机）
      if (typeof Platform.Version === "number" && Platform.Version >= 31) {
        permissions.push(PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT);
      }


      const result = await PermissionsAndroid.requestMultiple(permissions as any);
      

      const cameraGranted = result[PermissionsAndroid.PERMISSIONS.CAMERA] === PermissionsAndroid.RESULTS.GRANTED;
      const microphoneGranted = result[PermissionsAndroid.PERMISSIONS.RECORD_AUDIO] === PermissionsAndroid.RESULTS.GRANTED;

      const allGranted = cameraGranted && microphoneGranted;

      if (!allGranted) {
        console.warn("[CameraPermissionService] Camera or microphone permission denied");
      }

      return {
        camera: cameraGranted,
        microphone: microphoneGranted,
        allGranted
      };
    } catch (error) {
      console.error("[CameraPermissionService] Error requesting permissions:", error);
      return {
        camera: false,
        microphone: false,
        allGranted: false
      };
    }
  }

  /**
   * 检查摄像头权限（仅检查，不请求）
   */
  async hasCameraPermission(): Promise<boolean> {
    if (Platform.OS === "android") {
      try {
        const granted = await PermissionsAndroid.check(PermissionsAndroid.PERMISSIONS.CAMERA);
        return granted;
      } catch (error) {
        console.error("[CameraPermissionService] Error checking camera permission:", error);
        return false;
      }
    }
    
    // iOS 无法预先检查，返回 true
    return true;
  }

  /**
   * 检查麦克风权限（仅检查，不请求）
   */
  async hasMicrophonePermission(): Promise<boolean> {
    if (Platform.OS === "android") {
      try {
        const granted = await PermissionsAndroid.check(PermissionsAndroid.PERMISSIONS.RECORD_AUDIO);
        return granted;
      } catch (error) {
        console.error("[CameraPermissionService] Error checking microphone permission:", error);
        return false;
      }
    }
    
    // iOS 无法预先检查，返回 true
    return true;
  }

  /**
   * 显示权限被拒绝的提示
   */
  showPermissionDeniedAlert(missingPermissions: string[]): void {
    const message = `需要以下权限才能进行视频通话：\n${missingPermissions.join("、")}\n\n请在系统设置中授予权限。`;
    
    Alert.alert(
      "权限不足 / Permission Required",
      message,
      [
        {
          text: "确定 / OK",
          style: "default"
        }
      ]
    );
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
