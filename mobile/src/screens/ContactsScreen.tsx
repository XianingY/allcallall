import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  View,
  Text,
  StyleSheet,
  FlatList,
  RefreshControl,
  Alert,
  Modal,
  TouchableOpacity,
  Pressable
} from "react-native";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { useFocusEffect } from "@react-navigation/native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { useAuthContext } from "../context/AuthContext";
import {
  listContacts,
  addContact,
  removeContact,
  fetchPresence,
  searchUsers,
  User,
  PresenceRecord
} from "../api/users";
import {
  createAbuseReport,
  createBlock,
  fetchCallHistory
} from "../api/commercial";
import ContactListItem from "../components/ContactListItem";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import PresenceBadge from "../components/PresenceBadge";
import { useCommercial } from "../context/CommercialContext";
import { useSignaling } from "../context/SignalingContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import AnalyticsService from "../services/AnalyticsService";
import {
  FIRST_CALL_STARTED_STORAGE_KEY,
  FIRST_TRANSLATION_ENABLED_STORAGE_KEY,
  ONBOARDING_DISMISSED_STORAGE_KEY
} from "../constants/onboarding";
import { findTranslationUsage } from "../utils/usage";

type Props = NativeStackScreenProps<RootStackParamList, "Contacts">;

type ReportCategory =
  | "spam"
  | "harassment"
  | "impersonation"
  | "fraud"
  | "sexual_content"
  | "other";

interface ReportCategoryOption {
  value: ReportCategory;
  label: string;
  description: string;
}

const REPORT_CATEGORIES: ReportCategoryOption[] = [
  { value: "spam", label: "垃圾信息", description: "广告、反复骚扰或批量消息" },
  { value: "harassment", label: "骚扰辱骂", description: "辱骂、威胁或持续骚扰" },
  { value: "impersonation", label: "冒充身份", description: "假冒他人或伪装官方身份" },
  { value: "fraud", label: "诈骗欺诈", description: "诱导转账、钓鱼或其他诈骗行为" },
  { value: "sexual_content", label: "性相关内容", description: "不当性暗示、露骨内容或骚扰" },
  { value: "other", label: "其他问题", description: "不属于以上分类，但仍需处理" }
];

const ContactsScreen: React.FC<Props> = ({ navigation }) => {
  const { user, token, logout } = useAuthContext();
  const { usage } = useCommercial();
  const { startCall, connectionReady } = useSignaling();

  const [contacts, setContacts] = useState<User[]>([]);
  const [presence, setPresence] = useState<Record<string, PresenceRecord>>({});
  const [loadingContacts, setLoadingContacts] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [isAddModalVisible, setAddModalVisible] = useState(false);
  const [newContactEmail, setNewContactEmail] = useState("");
  const [searchResults, setSearchResults] = useState<User[]>([]);
  const [selectedContact, setSelectedContact] = useState<User | null>(null);
  const [checklistDismissed, setChecklistDismissed] = useState(false);
  const [reportTarget, setReportTarget] = useState<User | null>(null);
  const [reportCategory, setReportCategory] = useState<ReportCategory>("harassment");
  const [reportDetails, setReportDetails] = useState("");
  const [hasCallHistory, setHasCallHistory] = useState(false);
  const [submittingReport, setSubmittingReport] = useState(false);

  const loadContacts = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      setLoadingContacts(true);
      const data = await listContacts(token);
      setContacts(data);
    } catch (error) {
      console.error(error);
      Alert.alert("拉取联系人失败 / Failed to load contacts", "无法加载联系人列表，请重试 / Please try again later.");
    } finally {
      setLoadingContacts(false);
    }
  }, [token]);

  const loadPresence = useCallback(async () => {
    if (!token) {
      return;
    }
    const emails = [user?.email, ...contacts.map((c) => c.email)].filter(
      Boolean
    ) as string[];

    if (!emails.length) {
      return;
    }

    try {
      const presenceList = await fetchPresence(token, emails);
      const map: Record<string, PresenceRecord> = {};
      presenceList.forEach((record) => {
        map[record.email] = record;
      });
      setPresence(map);
    } catch (error) {
      console.warn("presence load failed", error);
    }
  }, [contacts, token, user?.email]);

  useEffect(() => {
    loadContacts();
  }, [loadContacts]);

  useEffect(() => {
    const interval = setInterval(loadPresence, 10000);
    return () => clearInterval(interval);
  }, [loadPresence]);

  useEffect(() => {
    loadPresence();
  }, [contacts, loadPresence]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await loadContacts();
    await loadPresence();
    if (token) {
      try {
        const calls = await fetchCallHistory(token, 365);
        setHasCallHistory(calls.length > 0);
      } catch {
        // Ignore onboarding history failures during manual refresh.
      }
    }
    setRefreshing(false);
  }, [loadContacts, loadPresence, token]);

  const [hasStartedFirstCall, setHasStartedFirstCall] = useState(false);
  const [hasEnabledTranslation, setHasEnabledTranslation] = useState(false);

  const loadOnboardingState = useCallback(async () => {
    try {
      const [dismissed, firstCall, firstTranslation] = await Promise.all([
        AsyncStorage.getItem(ONBOARDING_DISMISSED_STORAGE_KEY),
        AsyncStorage.getItem(FIRST_CALL_STARTED_STORAGE_KEY),
        AsyncStorage.getItem(FIRST_TRANSLATION_ENABLED_STORAGE_KEY)
      ]);
      setChecklistDismissed(dismissed === "dismissed");
      setHasStartedFirstCall(firstCall === "true");
      setHasEnabledTranslation(firstTranslation === "true");
    } catch {
      // Ignore onboarding storage failures.
    }

    if (!token) {
      setHasCallHistory(false);
      return;
    }

    try {
      const calls = await fetchCallHistory(token, 365);
      setHasCallHistory(calls.length > 0);
    } catch {
      // Ignore onboarding history failures.
    }
  }, [token]);

  useFocusEffect(
    useCallback(() => {
      void loadOnboardingState();
    }, [loadOnboardingState])
  );

  const handleAddContact = useCallback(async () => {
    if (!token) {
      return;
    }
    const target = newContactEmail.trim().toLowerCase();
    if (!target) {
      return;
    }

    try {
      const isFirstContact = contacts.length === 0;
      await addContact(token, target);
      if (isFirstContact) {
        AnalyticsService.track("first_contact_added");
      }
      setNewContactEmail("");
      setSearchResults([]);
      setAddModalVisible(false);
      await loadContacts();
      await loadPresence();
      Alert.alert("联系人已添加 / Contact Added", `${target} 已加入联系人 / ${target} has been added to your contacts.`);
    } catch (error) {
      console.error(error);
      Alert.alert("添加失败 / Failed to add", "无法添加联系人，可能已存在或输入有误 / This contact may already exist or the email is invalid.");
    }
  }, [contacts.length, loadContacts, loadPresence, newContactEmail, token]);

  const handleSearchContact = useCallback(async () => {
    if (!token) {
      return;
    }
    const query = newContactEmail.trim().toLowerCase();
    if (!query) {
      setSearchResults([]);
      return;
    }

    try {
      const results = await searchUsers(token, query);
      setSearchResults(results.slice(0, 5));
    } catch (error) {
      console.warn("[ContactsScreen] Failed to search contacts:", error);
      setSearchResults([]);
    }
  }, [newContactEmail, token]);

  const handleRemoveContact = useCallback(
    (contact: User) => {
      Alert.alert(
        "删除联系人 / Delete Contact",
        `确定删除 ${contact.display_name || contact.email} 吗？ / Are you sure you want to delete ${contact.display_name || contact.email}?`,
        [
          { text: "取消 / Cancel", style: "cancel" },
          {
            text: "删除 / Delete",
            style: "destructive",
            onPress: async () => {
              if (!token) return;
              try {
                await removeContact(token, contact.id);
                await loadContacts();
                await loadPresence();
              } catch (error) {
                console.error(error);
                Alert.alert("删除失败 / Failed to delete", "请稍后再试 / Please try again later.");
              }
            }
          }
        ]
      );
    },
    [loadContacts, loadPresence, token]
  );

  const handleBlockUser = useCallback(
    (contact: User) => {
      if (!token) {
        return;
      }
      Alert.alert(
        "拉黑用户 / Block User",
        `拉黑后 ${contact.display_name || contact.email} 将无法搜索、加联系人或呼叫你。`,
        [
          { text: "取消", style: "cancel" },
          {
            text: "确认拉黑",
            style: "destructive",
            onPress: async () => {
              try {
                await createBlock(token, contact.id);
                await removeContact(token, contact.id);
                await loadContacts();
                Alert.alert("已拉黑", "该用户已被加入黑名单。");
              } catch (error) {
                console.error("[ContactsScreen] Failed to block user:", error);
                Alert.alert("操作失败", "无法拉黑该用户。");
              }
            }
          }
        ]
      );
    },
    [loadContacts, token]
  );

  const openReportModal = useCallback((contact: User) => {
    setReportTarget(contact);
    setReportCategory("harassment");
    setReportDetails("");
  }, []);

  const handleStartCall = useCallback(
    (email: string) => {
      if (!connectionReady) {
        Alert.alert("正在重新连接", "信令服务暂时不可用，请稍后再试。");
        return;
      }
      AnalyticsService.track("call_started");
      startCall(email);
    },
    [connectionReady, startCall]
  );

  const dismissChecklist = useCallback(async () => {
    setChecklistDismissed(true);
    try {
      await AsyncStorage.setItem(ONBOARDING_DISMISSED_STORAGE_KEY, "dismissed");
    } catch {
      // Ignore onboarding storage failures.
    }
  }, []);

  const translationUsage = useMemo(() => findTranslationUsage(usage), [usage]);
  const translationUsed = (translationUsage?.used_units ?? 0) > 0 || hasEnabledTranslation;

  const checklistItems = useMemo(() => {
    const hasContacts = contacts.length > 0;
    return [
      { key: "contact", label: "添加第一个联系人", done: hasContacts },
      { key: "call", label: "发起第一通通话", done: hasStartedFirstCall || hasCallHistory },
      { key: "translation", label: "首次启用实时翻译", done: translationUsed }
    ];
  }, [contacts.length, hasCallHistory, hasStartedFirstCall, translationUsed]);

  const handleSubmitReport = useCallback(async () => {
    if (!token || !reportTarget) {
      return;
    }
    try {
      setSubmittingReport(true);
      await createAbuseReport(token, {
        reported_user_id: reportTarget.id,
        category: reportCategory,
        details:
          reportDetails.trim() ||
          `Reported from contacts list for ${reportTarget.email}`
      });
      setReportTarget(null);
      setReportCategory("harassment");
      setReportDetails("");
      Alert.alert("举报已提交", "支持团队会根据记录进行处理。");
    } catch (error) {
      console.error("[ContactsScreen] Failed to report user:", error);
      Alert.alert("提交失败", "当前无法提交举报。");
    } finally {
      setSubmittingReport(false);
    }
  }, [reportCategory, reportDetails, reportTarget, token]);

  const handleContactActions = useCallback((contact: User) => {
    setSelectedContact(contact);
  }, []);

  const sortedContacts = useMemo(
    () =>
      [...contacts].sort((a, b) =>
        (a.display_name || a.email).localeCompare(
          b.display_name || b.email,
          "en"
        )
      ),
    [contacts]
  );

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <View>
          <Text style={styles.greeting}>你好, {user?.display_name || ""}</Text>
          <Text style={styles.subtitle}>{user?.email}</Text>
        </View>
        <View style={styles.headerButtons}>
          <TouchableOpacity
            style={styles.settingsButton}
            onPress={() => navigation.navigate("CallHistory")}
          >
            <Text style={styles.settingsText}>最近通话</Text>
          </TouchableOpacity>
          <TouchableOpacity
            style={styles.settingsButton}
            onPress={() => navigation.navigate("Settings")}
          >
            <Text style={styles.settingsText}>设置 / Settings</Text>
          </TouchableOpacity>
          <TouchableOpacity
            style={styles.changePasswordButton}
            onPress={() => navigation.navigate("ChangePassword")}
          >
            <Text style={styles.changePasswordText}>改密码 / Change Password</Text>
          </TouchableOpacity>
          <TouchableOpacity
            style={styles.changePasswordButton}
            onPress={() => navigation.navigate("Subscription")}
          >
            <Text style={styles.changePasswordText}>Premium</Text>
          </TouchableOpacity>
          <TouchableOpacity style={styles.logoutButton} onPress={logout}>
            <Text style={styles.logoutText}>退出登录 / Logout</Text>
          </TouchableOpacity>
        </View>
      </View>

      <View style={styles.presenceCard}>
        <Text style={styles.sectionTitle}>我的状态 / My Presence</Text>
        <PresenceBadge
          online={presence[user?.email ?? ""]?.online ?? false}
          lastSeen={presence[user?.email ?? ""]?.last_seen ?? null}
        />
      </View>

      {!checklistDismissed ? (
        <View style={styles.onboardingCard}>
          <View style={styles.onboardingHeader}>
            <Text style={styles.sectionTitle}>首日引导 / Onboarding</Text>
            <TouchableOpacity onPress={() => void dismissChecklist()}>
              <Text style={styles.dismissText}>隐藏</Text>
            </TouchableOpacity>
          </View>
          {checklistItems.map((item) => (
            <View key={item.key} style={styles.checklistRow}>
              <Text style={[styles.checklistDot, item.done && styles.checklistDotDone]}>
                {item.done ? "●" : "○"}
              </Text>
              <Text style={[styles.checklistText, item.done && styles.checklistTextDone]}>
                {item.label}
              </Text>
            </View>
          ))}
        </View>
      ) : null}

      <View style={styles.sectionHeader}>
        <Text style={styles.sectionTitle}>联系人 / Contacts</Text>
        <PrimaryButton
          title="添加联系人"
          onPress={() => setAddModalVisible(true)}
          style={styles.addButton}
        />
      </View>

      <FlatList
        data={sortedContacts}
        keyExtractor={(item) => String(item.id)}
        renderItem={({ item }) => (
          <ContactListItem
            contact={item}
            presence={presence[item.email]}
            onCall={handleStartCall}
            onPressActions={handleContactActions}
          />
        )}
        contentContainerStyle={styles.listContent}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} />
        }
        ListEmptyComponent={
          !loadingContacts ? (
            <Text style={styles.emptyText}>
              还没有联系人，点击"添加联系人"开始吧 / No contacts yet. Click "Add" to get started.
            </Text>
          ) : null
        }
      />

      <Modal
        visible={isAddModalVisible}
        transparent
        animationType="slide"
        onRequestClose={() => setAddModalVisible(false)}
      >
        <View style={styles.modalBackdrop}>
          <View style={styles.modalContent}>
            <Text style={styles.modalTitle}>添加联系人 / Add Contact</Text>
            <TextField
              label="邮箱 / Email Address"
              autoCapitalize="none"
              keyboardType="email-address"
              value={newContactEmail}
              onChangeText={setNewContactEmail}
            />
            <PrimaryButton title="搜索用户" onPress={() => void handleSearchContact()} style={styles.searchButton} />
            {searchResults.length > 0 ? (
              <View style={styles.searchResults}>
                {searchResults.map((result) => (
                  <Pressable
                    key={result.id}
                    style={styles.searchResultRow}
                    onPress={() => setNewContactEmail(result.email)}
                  >
                    <Text style={styles.searchResultTitle}>{result.display_name || result.email}</Text>
                    <Text style={styles.searchResultMeta}>{result.email}</Text>
                  </Pressable>
                ))}
              </View>
            ) : null}
            <PrimaryButton title="添加 / Add" onPress={handleAddContact} />
            <PrimaryButton
              title="取消 / Cancel"
              onPress={() => setAddModalVisible(false)}
              style={styles.modalCancel}
            />
          </View>
        </View>
      </Modal>

      <Modal
        visible={Boolean(selectedContact)}
        transparent
        animationType="fade"
        onRequestClose={() => setSelectedContact(null)}
      >
        <Pressable style={styles.modalBackdrop} onPress={() => setSelectedContact(null)}>
          <Pressable style={styles.actionSheet}>
            <Text style={styles.modalTitle}>{selectedContact?.display_name || selectedContact?.email}</Text>
            <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
              if (selectedContact) {
                handleStartCall(selectedContact.email);
              }
              setSelectedContact(null);
            }}>
              <Text style={styles.actionSheetText}>呼叫 / Call</Text>
            </TouchableOpacity>
            <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
              if (selectedContact) {
                openReportModal(selectedContact);
              }
              setSelectedContact(null);
            }}>
              <Text style={styles.actionSheetText}>举报 / Report</Text>
            </TouchableOpacity>
            <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
              if (selectedContact) {
                handleBlockUser(selectedContact);
              }
              setSelectedContact(null);
            }}>
              <Text style={[styles.actionSheetText, styles.destructiveText]}>拉黑 / Block</Text>
            </TouchableOpacity>
            <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
              if (selectedContact) {
                handleRemoveContact(selectedContact);
              }
              setSelectedContact(null);
            }}>
              <Text style={[styles.actionSheetText, styles.destructiveText]}>删除 / Remove</Text>
            </TouchableOpacity>
          </Pressable>
        </Pressable>
      </Modal>

      <Modal
        visible={Boolean(reportTarget)}
        transparent
        animationType="slide"
        onRequestClose={() => setReportTarget(null)}
      >
        <View style={styles.modalBackdrop}>
          <View style={styles.modalContent}>
            <Text style={styles.modalTitle}>
              举报 {reportTarget?.display_name || reportTarget?.email}
            </Text>
            <Text style={styles.reportDescription}>
              选择最接近的问题类型，支持团队会按照分类处理。
            </Text>
            <View style={styles.reportCategoryList}>
              {REPORT_CATEGORIES.map((item) => {
                const selected = item.value === reportCategory;
                return (
                  <Pressable
                    key={item.value}
                    style={[
                      styles.reportCategoryCard,
                      selected && styles.reportCategoryCardSelected
                    ]}
                    onPress={() => setReportCategory(item.value)}
                  >
                    <Text
                      style={[
                        styles.reportCategoryTitle,
                        selected && styles.reportCategoryTitleSelected
                      ]}
                    >
                      {item.label}
                    </Text>
                    <Text
                      style={[
                        styles.reportCategoryMeta,
                        selected && styles.reportCategoryMetaSelected
                      ]}
                    >
                      {item.description}
                    </Text>
                  </Pressable>
                );
              })}
            </View>
            <TextField
              label="补充说明 / Details"
              multiline
              numberOfLines={4}
              value={reportDetails}
              onChangeText={setReportDetails}
              style={styles.reportDetailsInput}
              placeholder="选填，补充发生了什么 / Optional details"
            />
            <PrimaryButton
              title={submittingReport ? "提交中..." : "提交举报"}
              onPress={() => void handleSubmitReport()}
              disabled={submittingReport}
            />
            <PrimaryButton
              title="取消 / Cancel"
              onPress={() => {
                setReportTarget(null);
                setReportDetails("");
              }}
              style={styles.modalCancel}
              disabled={submittingReport}
            />
          </View>
        </View>
      </Modal>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f3f4f6",
    paddingTop: 48,
    paddingHorizontal: 20
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 24
  },
  greeting: {
    fontSize: 24,
    fontWeight: "700",
    color: "#111827"
  },
  subtitle: {
    marginTop: 4,
    color: "#6b7280"
  },
  headerButtons: {
    gap: 8
  },
  settingsButton: {
    backgroundColor: "#10b981",
    paddingVertical: 10,
    paddingHorizontal: 12,
    borderRadius: 10
  },
  settingsText: {
    color: "#fff",
    fontWeight: "600",
    fontSize: 12
  },
  changePasswordButton: {
    backgroundColor: "#3b82f6",
    paddingVertical: 10,
    paddingHorizontal: 12,
    borderRadius: 10
  },
  changePasswordText: {
    color: "#fff",
    fontWeight: "600",
    fontSize: 12
  },
  logoutButton: {
    backgroundColor: "#e5e7eb",
    paddingVertical: 10,
    paddingHorizontal: 14,
    borderRadius: 10
  },
  logoutText: {
    color: "#111827",
    fontWeight: "600"
  },
  presenceCard: {
    backgroundColor: "#fff",
    padding: 18,
    borderRadius: 16,
    marginBottom: 24
  },
  onboardingCard: {
    backgroundColor: "#fff",
    padding: 18,
    borderRadius: 16,
    marginBottom: 18
  },
  onboardingHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 8
  },
  dismissText: {
    color: "#2563eb",
    fontWeight: "700"
  },
  checklistRow: {
    flexDirection: "row",
    alignItems: "center",
    marginTop: 10
  },
  checklistDot: {
    width: 18,
    color: "#94a3b8"
  },
  checklistDotDone: {
    color: "#16a34a"
  },
  checklistText: {
    color: "#334155"
  },
  checklistTextDone: {
    color: "#16a34a",
    fontWeight: "700"
  },
  sectionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 12
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "600",
    color: "#111827"
  },
  addButton: {
    paddingHorizontal: 16,
    paddingVertical: 10
  },
  listContent: {
    paddingBottom: 140
  },
  emptyText: {
    textAlign: "center",
    color: "#6b7280",
    marginTop: 40
  },
  modalBackdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.3)",
    justifyContent: "center",
    paddingHorizontal: 20
  },
  modalContent: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 24
  },
  modalTitle: {
    fontSize: 20,
    fontWeight: "700",
    marginBottom: 16
  },
  modalCancel: {
    marginTop: 12,
    backgroundColor: "#9ca3af"
  },
  searchButton: {
    marginBottom: 12
  },
  searchResults: {
    marginBottom: 12
  },
  searchResultRow: {
    borderWidth: 1,
    borderColor: "#e5e7eb",
    borderRadius: 12,
    padding: 12,
    marginBottom: 8
  },
  searchResultTitle: {
    fontWeight: "700",
    color: "#0f172a"
  },
  searchResultMeta: {
    color: "#64748b",
    marginTop: 4
  },
  reportDescription: {
    color: "#475569",
    marginBottom: 16,
    lineHeight: 20
  },
  reportCategoryList: {
    gap: 10,
    marginBottom: 12
  },
  reportCategoryCard: {
    borderWidth: 1,
    borderColor: "#cbd5e1",
    borderRadius: 14,
    padding: 14,
    backgroundColor: "#f8fafc"
  },
  reportCategoryCardSelected: {
    borderColor: "#2563eb",
    backgroundColor: "#dbeafe"
  },
  reportCategoryTitle: {
    fontWeight: "700",
    color: "#0f172a"
  },
  reportCategoryTitleSelected: {
    color: "#1d4ed8"
  },
  reportCategoryMeta: {
    marginTop: 4,
    color: "#64748b"
  },
  reportCategoryMetaSelected: {
    color: "#1e3a8a"
  },
  reportDetailsInput: {
    minHeight: 96,
    textAlignVertical: "top",
    paddingTop: 12
  },
  actionSheet: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18
  },
  actionSheetButton: {
    paddingVertical: 14
  },
  actionSheetText: {
    fontSize: 16,
    fontWeight: "600",
    color: "#0f172a"
  },
  destructiveText: {
    color: "#dc2626"
  }
});

export default ContactsScreen;
