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
import AudioService from "../services/AudioService";

type Props = NativeStackScreenProps<RootStackParamList, "Settings">;

const SettingsScreen: React.FC<Props> = ({ navigation }) => {
  const { settings, updateAudioNotifications } = useSettings();

  const handleAudioToggle = (value: boolean) => {
    updateAudioNotifications(value);
    AudioService.setEnabled(value);
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

          <View style={styles.infoBox}>
            <Text style={styles.infoText}>
              ℹ️ 音频提醒在设备静音模式下不会播放
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
