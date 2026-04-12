import React from "react";
import {
  View,
  Text,
  StyleSheet,
  Switch,
  ScrollView,
  SafeAreaView,
  TextInput,
  TouchableOpacity
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import { RootStackParamList } from "../navigation/AppNavigator";
import { useCommercial } from "../context/CommercialContext";
import { useSettings } from "../context/SettingsContext";
import type { VideoQuality } from "../services/VideoService";
import AudioService from "../services/AudioServiceExpo";
import VibrationService from "../services/VibrationService";

type Props = NativeStackScreenProps<RootStackParamList, "Settings">;

const SettingsScreen: React.FC<Props> = ({ navigation }) => {
  const { 
    settings, 
    updateAudioNotifications, 
    updateVibration, 
    updatePushNotifications,
    updateDefaultVideoEnabled,
    updateDefaultAudioEnabled,
    updateCameraFacing,
    updateVideoQuality,
    updateVideoMaxBitrateKbps,
    updateVideoAdaptiveBitrateEnabled
  } = useSettings();
  const { tier, usage } = useCommercial();

  const [bitrateInput, setBitrateInput] = React.useState(settings.videoMaxBitrateKbps.toString());

  React.useEffect(() => {
    setBitrateInput(settings.videoMaxBitrateKbps.toString());
  }, [settings.videoMaxBitrateKbps]);

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

  const handleBitrateBlur = () => {
    let val = parseInt(bitrateInput, 10);
    if (isNaN(val)) val = 900;
    if (val < 100) val = 100;
    if (val > 2500) val = 2500;
    
    setBitrateInput(val.toString());
    updateVideoMaxBitrateKbps(val);
  };

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView style={styles.scrollView}>
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>商业与合规 / Commercial</Text>

          <TouchableOpacity style={styles.linkRow} onPress={() => navigation.navigate("Subscription")}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>订阅与权益 / Subscription</Text>
              <Text style={styles.settingDescription}>
                当前 {tier === "premium" ? "Premium" : "Free"} · {usage[0]?.unlimited ? "翻译不限量" : `翻译剩余 ${usage[0]?.remaining_units ?? "--"} 分钟`}
              </Text>
            </View>
          </TouchableOpacity>

          <TouchableOpacity style={styles.linkRow} onPress={() => navigation.navigate("BlockedUsers")}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>黑名单 / Blocked Users</Text>
              <Text style={styles.settingDescription}>
                管理你主动拉黑的用户。
              </Text>
            </View>
          </TouchableOpacity>

          <TouchableOpacity style={styles.linkRow} onPress={() => navigation.navigate("Legal")}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>法律文档 / Legal</Text>
              <Text style={styles.settingDescription}>
                查看隐私政策、条款、支持邮箱和账号删除路径。
              </Text>
            </View>
          </TouchableOpacity>

          <TouchableOpacity style={[styles.linkRow, styles.dangerRow]} onPress={() => navigation.navigate("DeleteAccount")}>
            <View style={styles.settingInfo}>
              <Text style={[styles.settingTitle, styles.dangerText]}>删除账号 / Delete Account</Text>
              <Text style={styles.settingDescription}>
                永久删除当前账号及关联数据。
              </Text>
            </View>
          </TouchableOpacity>
        </View>

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
                控制客户端提醒与权限申请；不会停止服务端推送投递
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

          <View style={styles.settingItem}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>自适应码率 / Adaptive Bitrate</Text>
              <Text style={styles.settingDescription}>
                根据网络状况自动调整视频质量
              </Text>
            </View>
            <Switch
              trackColor={{ false: "#767577", true: "#81b0ff" }}
              thumbColor={settings.videoAdaptiveBitrateEnabled ? "#f5dd4b" : "#f4f3f4"}
              onValueChange={updateVideoAdaptiveBitrateEnabled}
              value={settings.videoAdaptiveBitrateEnabled}
            />
          </View>

          <View style={styles.settingItem}>
            <View style={styles.settingInfo}>
              <Text style={styles.settingTitle}>视频最大码率 / Max Bitrate (kbps)</Text>
              <Text style={styles.settingDescription}>
                限制最大视频带宽 (100-2500)
              </Text>
            </View>
            <TextInput
              style={styles.input}
              keyboardType="numeric"
              value={bitrateInput}
              onChangeText={setBitrateInput}
              onBlur={handleBitrateBlur}
              maxLength={4}
              returnKeyType="done"
            />
          </View>

          <View style={[styles.settingItem, { flexDirection: "column", alignItems: "stretch" }]}>
            <View style={{ marginBottom: 12 }}>
              <Text style={styles.settingTitle}>视频质量 / Video Quality</Text>
              <Text style={styles.settingDescription}>
                设置视频清晰度优先策略
              </Text>
            </View>
            <View style={styles.qualityContainer}>
              {(["low", "medium", "high"] as VideoQuality[]).map((q) => (
                <TouchableOpacity
                  key={q}
                  style={[
                    styles.qualityButton,
                    settings.videoQuality === q && styles.qualityButtonActive
                  ]}
                  onPress={() => updateVideoQuality(q)}
                >
                  <Text style={[
                    styles.qualityButtonText,
                    settings.videoQuality === q && styles.qualityButtonTextActive
                  ]}>
                    {q === "low" ? "Low" : q === "medium" ? "Medium" : "High"}
                  </Text>
                </TouchableOpacity>
              ))}
            </View>
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
  linkRow: {
    backgroundColor: "#f8fafc",
    borderRadius: 12,
    paddingHorizontal: 14,
    paddingVertical: 14,
    marginBottom: 12
  },
  dangerRow: {
    borderWidth: 1,
    borderColor: "#fecaca"
  },
  dangerText: {
    color: "#b91c1c"
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
  },
  input: {
    height: 40,
    borderWidth: 1,
    borderColor: "#d1d5db",
    borderRadius: 6,
    paddingHorizontal: 12,
    minWidth: 80,
    textAlign: "center",
    color: "#111827",
    backgroundColor: "#fff"
  },
  qualityContainer: {
    flexDirection: "row",
    justifyContent: "space-between",
    marginTop: 4
  },
  qualityButton: {
    flex: 1,
    paddingVertical: 8,
    alignItems: "center",
    borderRadius: 6,
    borderWidth: 1,
    borderColor: "#e5e7eb",
    marginHorizontal: 4,
    backgroundColor: "#f9fafb"
  },
  qualityButtonActive: {
    backgroundColor: "#eff6ff",
    borderColor: "#3b82f6"
  },
  qualityButtonText: {
    fontSize: 14,
    color: "#4b5563",
    fontWeight: "500"
  },
  qualityButtonTextActive: {
    color: "#2563eb",
    fontWeight: "600"
  }
});

export default SettingsScreen;
