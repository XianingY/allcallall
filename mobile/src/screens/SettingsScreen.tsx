import React from "react";
import {
  View,
  Text,
  StyleSheet,
  Switch,
  ScrollView,
  SafeAreaView
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import { RootStackParamList } from "../navigation/AppNavigator";
import { useSettings } from "../context/SettingsContext";
import AudioService from "../services/AudioServiceExpo";
import VibrationService from "../services/VibrationService";
import PushNotificationService from "../services/PushNotificationService";

type Props = NativeStackScreenProps<RootStackParamList, "Settings">;

const SettingsScreen: React.FC<Props> = ({ navigation }) => {
  const { 
    settings, 
    updateAudioNotifications, 
    updateVibration, 
    updatePushNotifications,
    updateDefaultVideoEnabled,
    updateDefaultAudioEnabled,
    updateCameraFacing
  } = useSettings();

  const handleAudioToggle = (value: boolean) => {
    updateAudioNotifications(value);
    AudioService.setEnabled(value);
  };

  const handleVibrationToggle = (value: boolean) => {
    updateVibration(value);
    VibrationService.setEnabled(value);
  };

  const handlePushToggle = (value: boolean) => {
    updatePushNotifications(value);
    // 权限由 PushNotificationService 在初始化时自动管理
  };

  const handleVideoToggle = (value: boolean) => {
    updateDefaultVideoEnabled(value);
  };

  const handleMicToggle = (value: boolean) => {
    updateDefaultAudioEnabled(value);
  };

  const handleCameraFacingToggle = (value: boolean) => {
    updateCameraFacing(value ? "back" : "front");
  };

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView style={styles.scrollView}>
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>通话设置 / Call Settings</Text>

          <View style={styles.settingItem}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>音频提醒 / Audio Notifications</Text>
              <Text style={styles.settingDescription}>
                开启后，来电和拨出电话时会播放提示音
              </Text>
            </View>
            <Switch
              trackColor={{ false: "#767577", true: "#81b0ff" }}
              thumbColor={settings.audioNotificationsEnabled ? "#f5dd4b" : "#f4f3f4"}
              onValueChange={handleAudioToggle}
              value={settings.audioNotificationsEnabled}
            />
          </View>

          <View style={styles.settingItem}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>震动反馈 / Vibration</Text>
              <Text style={styles.settingDescription}>
                开启后，来电和拨号时会有震动提醒
              </Text>
            </View>
            <Switch
              trackColor={{ false: "#767577", true: "#81b0ff" }}
              thumbColor={settings.vibrationEnabled ? "#f5dd4b" : "#f4f3f4"}
              onValueChange={handleVibrationToggle}
              value={settings.vibrationEnabled}
            />
          </View>

          <View style={styles.settingItem}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>推送通知 / Push Notifications</Text>
              <Text style={styles.settingDescription}>
                开启后，即使应用在后台也能接收来电通知
              </Text>
            </View>
            <Switch
              trackColor={{ false: "#767577", true: "#81b0ff" }}
              thumbColor={settings.pushNotificationsEnabled ? "#f5dd4b" : "#f4f3f4"}
              onValueChange={handlePushToggle}
              value={settings.pushNotificationsEnabled}
            />
          </View>

          <View style={styles.infoBox}>
            <Text style={styles.infoText}>
              ℹ️ 音频提醒在设备静音模式下不会播放，但震动仍会工作
            </Text>
          </View>
        </View>

        <View style={styles.section}>
          <Text style={styles.sectionTitle}>视频通话设置 / Video Call Settings</Text>

          <View style={styles.settingItem}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>默认开启视频 / Default Video On</Text>
              <Text style={styles.settingDescription}>
                开启后，通话时默认启用摄像头
              </Text>
            </View>
            <Switch
              trackColor={{ false: "#767577", true: "#81b0ff" }}
              thumbColor={settings.defaultVideoEnabled ? "#f5dd4b" : "#f4f3f4"}
              onValueChange={handleVideoToggle}
              value={settings.defaultVideoEnabled}
            />
          </View>

          <View style={styles.settingItem}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>默认开启麦克风 / Default Microphone On</Text>
              <Text style={styles.settingDescription}>
                开启后，通话时默认启用麦克风
              </Text>
            </View>
            <Switch
              trackColor={{ false: "#767577", true: "#81b0ff" }}
              thumbColor={settings.defaultAudioEnabled ? "#f5dd4b" : "#f4f3f4"}
              onValueChange={handleMicToggle}
              value={settings.defaultAudioEnabled}
            />
          </View>

          <View style={styles.settingItem}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>默认摄像头 / Default Camera</Text>
              <Text style={styles.settingDescription}>
                {settings.cameraFacing === "front" ? "前置摄像头 / Front Camera" : "后置摄像头 / Back Camera"}
              </Text>
            </View>
            <Switch
              trackColor={{ false: "#767577", true: "#81b0ff" }}
              thumbColor={settings.cameraFacing === "back" ? "#f5dd4b" : "#f4f3f4"}
              onValueChange={handleCameraFacingToggle}
              value={settings.cameraFacing === "back"}
            />
          </View>

          <View style={styles.infoBox}>
            <Text style={styles.infoText}>
              ℹ️ 视频和麦克风可在通话过程中随时开关
            </Text>
          </View>
        </View>

        <View style={styles.section}>
          <Text style={styles.sectionTitle}>关于 / About</Text>
          <View style={styles.aboutItem}>
            <Text style={styles.aboutLabel}>版本 / Version</Text>
            <Text style={styles.aboutValue}>1.0.0</Text>
          </View>
          <View style={styles.aboutItem}>
            <Text style={styles.aboutLabel}>开发者 / Developer</Text>
            <Text style={styles.aboutValue}>AllCallAll Team</Text>
          </View>
        </View>

        <View style={styles.footer}>
          <Text style={styles.footerText}>© 2024 AllCallAll. All rights reserved.</Text>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f5f5f5"
  },
  scrollView: {
    flex: 1
  },
  section: {
    backgroundColor: "#fff",
    marginTop: 12,
    paddingHorizontal: 16,
    paddingVertical: 12
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#1f2937",
    marginBottom: 16
  },
  settingItem: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 12
  },
  settingInfo: {
    flex: 1,
    marginRight: 12
  },
  settingTitle: {
    fontSize: 16,
    fontWeight: "600",
    color: "#111827",
    marginBottom: 4
  },
  settingDescription: {
    fontSize: 13,
    color: "#6b7280",
    lineHeight: 18
  },
  infoBox: {
    backgroundColor: "#f0f9ff",
    padding: 12,
    borderRadius: 8,
    marginTop: 12
  },
  infoText: {
    fontSize: 13,
    color: "#0369a1"
  },
  aboutItem: {
    flexDirection: "row",
    justifyContent: "space-between",
    paddingVertical: 10,
    borderBottomWidth: 1,
    borderBottomColor: "#f3f4f6"
  },
  aboutLabel: {
    fontSize: 15,
    color: "#374151"
  },
  aboutValue: {
    fontSize: 15,
    color: "#111827",
    fontWeight: "500"
  },
  footer: {
    padding: 20,
    alignItems: "center"
  },
  footerText: {
    fontSize: 12,
    color: "#9ca3af"
  }
});

export default SettingsScreen;
