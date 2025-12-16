import React, { createContext, useContext, useState, useEffect } from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";

interface Settings {
  audioNotificationsEnabled: boolean;
  vibrationEnabled: boolean;
  pushNotificationsEnabled: boolean;
  // 视频通话设置
  defaultVideoEnabled: boolean; // 默认开启视频
  defaultAudioEnabled: boolean; // 默认开启麦克风
  cameraFacing: "front" | "back"; // 默认摄像头方向
}

interface SettingsContextValue {
  settings: Settings;
  updateAudioNotifications: (enabled: boolean) => void;
  updateVibration: (enabled: boolean) => void;
  updatePushNotifications: (enabled: boolean) => void;
  // 视频通话设置更新函数
  updateDefaultVideoEnabled: (enabled: boolean) => void;
  updateDefaultAudioEnabled: (enabled: boolean) => void;
  updateCameraFacing: (facing: "front" | "back") => void;
}

const SettingsContext = createContext<SettingsContextValue | undefined>(
  undefined
);

const SETTINGS_STORAGE_KEY = "@allcallall:settings";

export const SettingsProvider: React.FC<{ children: React.ReactNode }> = ({
  children
}) => {
  const [settings, setSettings] = useState<Settings>({
    audioNotificationsEnabled: true,
    vibrationEnabled: true,
    pushNotificationsEnabled: true,
    // 视频通话默认设置
    defaultVideoEnabled: false, // 默认关闭视频
    defaultAudioEnabled: true, // 默认开启麦克风
    cameraFacing: "front" // 默认前置摄像头
  });
  const [loaded, setLoaded] = useState(false);

  // 从本地存储加载设置
  useEffect(() => {
    const loadSettings = async () => {
      try {
        const storedSettings = await AsyncStorage.getItem(SETTINGS_STORAGE_KEY);
        if (storedSettings) {
          const parsed = JSON.parse(storedSettings);
          setSettings({
            audioNotificationsEnabled: parsed.audioNotificationsEnabled ?? true,
            vibrationEnabled: parsed.vibrationEnabled ?? true,
            pushNotificationsEnabled: parsed.pushNotificationsEnabled ?? true,
            defaultVideoEnabled: parsed.defaultVideoEnabled ?? false,
            defaultAudioEnabled: parsed.defaultAudioEnabled ?? true,
            cameraFacing: parsed.cameraFacing ?? "front"
          });
          console.log("[SettingsContext] Loaded settings from storage:", parsed);
        }
      } catch (error) {
        console.warn("[SettingsContext] Failed to load settings:", error);
      } finally {
        setLoaded(true);
      }
    };

    loadSettings();
  }, []);

  const updateAudioNotifications = async (enabled: boolean) => {
    const newSettings = {
      ...settings,
      audioNotificationsEnabled: enabled
    };
    setSettings(newSettings);
    await saveSettings(newSettings);
    console.log("[SettingsContext] Audio notifications updated:", enabled);
  };

  const updateVibration = async (enabled: boolean) => {
    const newSettings = {
      ...settings,
      vibrationEnabled: enabled
    };
    setSettings(newSettings);
    await saveSettings(newSettings);
    console.log("[SettingsContext] Vibration updated:", enabled);
  };

  const updatePushNotifications = async (enabled: boolean) => {
    const newSettings = {
      ...settings,
      pushNotificationsEnabled: enabled
    };
    setSettings(newSettings);
    await saveSettings(newSettings);
    console.log("[SettingsContext] Push notifications updated:", enabled);
  };

  const updateDefaultVideoEnabled = async (enabled: boolean) => {
    const newSettings = {
      ...settings,
      defaultVideoEnabled: enabled
    };
    setSettings(newSettings);
    await saveSettings(newSettings);
    console.log("[SettingsContext] Default video enabled updated:", enabled);
  };

  const updateDefaultAudioEnabled = async (enabled: boolean) => {
    const newSettings = {
      ...settings,
      defaultAudioEnabled: enabled
    };
    setSettings(newSettings);
    await saveSettings(newSettings);
    console.log("[SettingsContext] Default audio enabled updated:", enabled);
  };

  const updateCameraFacing = async (facing: "front" | "back") => {
    const newSettings = {
      ...settings,
      cameraFacing: facing
    };
    setSettings(newSettings);
    await saveSettings(newSettings);
    console.log("[SettingsContext] Camera facing updated:", facing);
  };

  const saveSettings = async (newSettings: Settings) => {
    try {
      await AsyncStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(newSettings));
      console.log("[SettingsContext] Settings saved:", newSettings);
    } catch (error) {
      console.warn("[SettingsContext] Failed to save settings:", error);
    }
  };

  if (!loaded) {
    // 可以返回加载指示器
    return null;
  }

  return (
    <SettingsContext.Provider
      value={{
        settings,
        updateAudioNotifications,
        updateVibration,
        updatePushNotifications,
        updateDefaultVideoEnabled,
        updateDefaultAudioEnabled,
        updateCameraFacing
      }}
    >
      {children}
    </SettingsContext.Provider>
  );
};

export const useSettings = () => {
  const context = useContext(SettingsContext);
  if (context === undefined) {
    throw new Error("useSettings must be used within a SettingsProvider");
  }
  return context;
};
