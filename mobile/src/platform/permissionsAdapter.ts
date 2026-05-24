import { Alert, PermissionsAndroid, Platform } from "react-native";

export interface PermissionResult {
  camera: boolean;
  microphone: boolean;
  allGranted: boolean;
}

export interface PermissionsAdapter {
  requestMeetingPermissions(): Promise<PermissionResult>;
  hasCameraPermission(): Promise<boolean>;
  hasMicrophonePermission(): Promise<boolean>;
  showPermissionDeniedAlert(missingPermissions: string[]): void;
}

const webAdapter: PermissionsAdapter = {
  async requestMeetingPermissions() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: true });
      stream.getTracks().forEach((track) => track.stop());
      return {
        camera: true,
        microphone: true,
        allGranted: true,
      };
    } catch (error) {
      console.warn("[PermissionsAdapter] Browser media permission denied:", error);
      return {
        camera: false,
        microphone: false,
        allGranted: false,
      };
    }
  },
  async hasCameraPermission() {
    return true;
  },
  async hasMicrophonePermission() {
    return true;
  },
  showPermissionDeniedAlert(missingPermissions) {
    Alert.alert(
      "权限不足 / Permission Required",
      `需要以下权限才能继续：\n${missingPermissions.join("、")}`
    );
  },
};

const nativeAdapter: PermissionsAdapter = {
  async requestMeetingPermissions() {
    if (Platform.OS !== "android") {
      return {
        camera: true,
        microphone: true,
        allGranted: true,
      };
    }

    try {
      const permissions: string[] = [
        PermissionsAndroid.PERMISSIONS.CAMERA,
        PermissionsAndroid.PERMISSIONS.RECORD_AUDIO,
      ];
      if (typeof Platform.Version === "number" && Platform.Version >= 31) {
        permissions.push(PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT);
      }

      const result = await PermissionsAndroid.requestMultiple(permissions as never);
      const camera = result[PermissionsAndroid.PERMISSIONS.CAMERA] === PermissionsAndroid.RESULTS.GRANTED;
      const microphone =
        result[PermissionsAndroid.PERMISSIONS.RECORD_AUDIO] === PermissionsAndroid.RESULTS.GRANTED;

      return {
        camera,
        microphone,
        allGranted: camera && microphone,
      };
    } catch (error) {
      console.error("[PermissionsAdapter] Failed to request Android permissions:", error);
      return {
        camera: false,
        microphone: false,
        allGranted: false,
      };
    }
  },
  async hasCameraPermission() {
    if (Platform.OS !== "android") {
      return true;
    }
    return PermissionsAndroid.check(PermissionsAndroid.PERMISSIONS.CAMERA);
  },
  async hasMicrophonePermission() {
    if (Platform.OS !== "android") {
      return true;
    }
    return PermissionsAndroid.check(PermissionsAndroid.PERMISSIONS.RECORD_AUDIO);
  },
  showPermissionDeniedAlert(missingPermissions) {
    Alert.alert(
      "权限不足 / Permission Required",
      `需要以下权限才能进行视频通话：\n${missingPermissions.join("、")}\n\n请在系统设置中授予权限。`
    );
  },
};

const permissionsAdapter: PermissionsAdapter =
  Platform.OS === "web" ? webAdapter : nativeAdapter;

export default permissionsAdapter;
