import React, { createContext, useContext, useState, useEffect } from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";

interface Settings {
  audioNotificationsEnabled: boolean;
  vibrationEnabled: boolean;
  pushNotificationsEnabled: boolean;
}

interface SettingsContextValue {
  settings: Settings;
  updateAudioNotifications: (enabled: boolean) => void;
  updateVibration: (enabled: boolean) => void;
  updatePushNotifications: (enabled: boolean) => void;
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
    pushNotificationsEnabled: true
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
            pushNotificationsEnabled: parsed.pushNotificationsEnabled ?? true
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
        updatePushNotifications
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
