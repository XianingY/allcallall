import { Alert, Linking, PermissionsAndroid, Platform } from "react-native";

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
      // iOS used to return "granted" without asking anything, so the app
      // reported success and then produced a call with no audio - the user
      // had no idea a permission was involved. Ask for the microphone for
      // real via expo-audio.
      //
      // Camera has to stay implicit: expo-camera is not a dependency, so the
      // iOS camera prompt comes from getUserMedia when the stream opens. That
      // means its status cannot be known up front, and a denial surfaces at
      // stream time.
      //
      // Imported lazily so the web build never pulls in expo-audio.
      try {
        const { requestRecordingPermissionsAsync } = await import("expo-audio");
        const audio = await requestRecordingPermissionsAsync();
        return {
          camera: true,
          microphone: audio.granted,
          allGranted: audio.granted,
        };
      } catch (error) {
        console.warn("[PermissionsAdapter] Failed to request iOS audio permission:", error);
        return { camera: true, microphone: false, allGranted: false };
      }
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
    // Offer a way out. Previously this only said "grant it in system
    // settings" with no button, so on iOS - where the permission is never
    // asked again once denied - the user was stuck: joining a meeting just
    // silently did nothing.
    Alert.alert(
      "权限不足 / Permission Required",
      `需要以下权限才能进行视频通话：\n${missingPermissions.join("、")}`,
      [
        { text: "取消 / Cancel", style: "cancel" },
        {
          text: "去设置 / Open Settings",
          onPress: () => {
            void Linking.openSettings().catch(() => {
              Alert.alert("无法打开设置 / Cannot open settings", "请手动在系统设置中授予权限。");
            });
          },
        },
      ]
    );
  },
};

const permissionsAdapter: PermissionsAdapter =
  Platform.OS === "web" ? webAdapter : nativeAdapter;

export default permissionsAdapter;
