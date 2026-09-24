import React, { useCallback, useMemo, useRef, useState } from "react";
import { Alert, View } from "react-native";
import { useFocusEffect } from "@react-navigation/native";
import axios from "axios";

import { addContact, removeContact, searchUsers, User } from "../api/users";
import { createBlock, fetchCallHistory } from "../api/commercial";
import ContactListItem from "../components/ContactListItem";
import { useAuthContext } from "../context/AuthContext";
import { useCommercial } from "../context/CommercialContext";
import { useFollowUps } from "../context/FollowUpContext";
import { useOrganization } from "../context/OrganizationContext";
import { useSettings } from "../context/SettingsContext";
import { useSignaling } from "../context/signalingContextValue";
import AnalyticsService from "../services/AnalyticsService";
import {
  AddContactModal,
  ContactActionSheet,
  ContactsList,
  ContactsOverview,
  ReportModal,
  styles,
  useContactsData,
  useOnboardingChecklist,
  useReportFlow
} from "./contacts";
import type { ContactsScreenProps as Props } from "./contacts";

// 本文件只保留状态与业务编排；展示部分已拆分到 ./contacts 下的
// ContactsOverview / ContactsList / ContactsModals（见 #24 拆分）。
const ContactsScreen: React.FC<Props> = ({ navigation }) => {
  const { user, token, logout } = useAuthContext();
  useCommercial();
  const { items: followUpItems, refreshFollowUps } = useFollowUps();
  const { currentOrganization } = useOrganization();
  const { settings } = useSettings();
  const {
    startCall,
    connectionReady,
    setTranslationLanguage,
    setTranslationSourceLanguage
  } = useSignaling();
  const sessionExpiredHandledRef = useRef(false);

  const [refreshing, setRefreshing] = useState(false);
  const [isAddModalVisible, setAddModalVisible] = useState(false);
  const [newContactEmail, setNewContactEmail] = useState("");
  const [searchResults, setSearchResults] = useState<User[]>([]);
  const [selectedContact, setSelectedContact] = useState<User | null>(null);
  const {
    reportTarget,
    reportCategory,
    setReportCategory,
    reportDetails,
    setReportDetails,
    submittingReport,
    openReportModal,
    closeReportModal,
    submitReport
  } = useReportFlow(token);

  const handleSessionExpired = useCallback(() => {
    if (sessionExpiredHandledRef.current) {
      return;
    }
    sessionExpiredHandledRef.current = true;
    Alert.alert(
      "登录已过期 / Session Expired",
      "登录状态已过期，请重新登录 / Your session has expired. Please log in again.",
      [
        {
          text: "重新登录 / Login Again",
          onPress: () => {
            void logout();
          }
        }
      ]
    );
  }, [logout]);

  const isUnauthorizedError = useCallback((error: unknown): boolean => {
    return axios.isAxiosError(error) && error.response?.status === 401;
  }, []);

  const { contacts, presence, loadingContacts, loadContacts, loadPresence } =
    useContactsData({
      token,
      userEmail: user?.email,
      onSessionExpired: handleSessionExpired,
      isUnauthorizedError
    });

  const {
    checklistDismissed,
    setHasCallHistory,
    dismissChecklist,
    markFirstBusinessContactTracked,
    checklistItems
  } = useOnboardingChecklist(token, contacts.length);

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
  }, [loadContacts, loadPresence, setHasCallHistory, token]);

  useFocusEffect(
    useCallback(() => {
      void refreshFollowUps();
    }, [refreshFollowUps])
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
        AnalyticsService.track("first_business_contact_added");
        markFirstBusinessContactTracked();
      }
      setNewContactEmail("");
      setSearchResults([]);
      setAddModalVisible(false);
      await loadContacts();
      await loadPresence();
      Alert.alert("联系人已添加 / Contact Added", `${target} 已加入联系人 / ${target} has been added to your contacts.`);
    } catch (error) {
      console.error(error);
      if (isUnauthorizedError(error)) {
        handleSessionExpired();
        return;
      }
      Alert.alert("添加失败 / Failed to add", "无法添加联系人，可能已存在或输入有误 / This contact may already exist or the email is invalid.");
    }
  }, [
    contacts.length,
    handleSessionExpired,
    isUnauthorizedError,
    loadContacts,
    loadPresence,
    markFirstBusinessContactTracked,
    newContactEmail,
    token
  ]);

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
                if (isUnauthorizedError(error)) {
                  handleSessionExpired();
                  return;
                }
                Alert.alert("删除失败 / Failed to delete", "请稍后再试 / Please try again later.");
              }
            }
          }
        ]
      );
    },
    [handleSessionExpired, isUnauthorizedError, loadContacts, loadPresence, token]
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

  const handleStartCallWithContact = useCallback(
    (contact: User) => {
      if (contact.profile?.default_source_lang) {
        setTranslationSourceLanguage(contact.profile.default_source_lang);
      }
      if (contact.profile?.default_target_lang) {
        setTranslationLanguage(contact.profile.default_target_lang);
      }
      handleStartCall(contact.email);
    },
    [handleStartCall, setTranslationLanguage, setTranslationSourceLanguage]
  );

  const followUpInbox = useMemo(() => {
    const overdue = followUpItems.filter((item) => item.is_overdue).length;
    const today = followUpItems.filter((item) => {
      if (!item.task.due_at || item.is_overdue) {
        return false;
      }
      return new Date(item.task.due_at).toDateString() === new Date().toDateString();
    }).length;
    return {
      overdue,
      today,
      topItems: followUpItems.slice(0, 3)
    };
  }, [followUpItems]);

  const handleContactActions = useCallback((contact: User) => {
    setSelectedContact(contact);
  }, []);

  const handleOpenDetail = useCallback((contact: User) => {
    navigation.navigate("ContactDetail", { contact });
  }, [navigation]);

  const handleQuickShareInvite = useCallback(async () => {
    navigation.navigate("Invitation");
  }, [navigation]);

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

  const renderContact = useCallback(
    ({ item }: { item: User }) => (
      <ContactListItem
        contact={item}
        presence={presence[item.email]}
        onCall={handleStartCallWithContact}
        onPressDetail={handleOpenDetail}
        onPressActions={handleContactActions}
      />
    ),
    [presence, handleStartCallWithContact, handleOpenDetail, handleContactActions]
  );

  const contactKeyExtractor = useCallback(
    (item: User) => String(item.id),
    []
  );

  return (
    <View style={styles.container}>
      <ContactsOverview
        navigation={navigation}
        user={user}
        currentOrganization={currentOrganization}
        logout={logout}
        presence={presence}
        checklistDismissed={checklistDismissed}
        checklistItems={checklistItems}
        onDismissChecklist={dismissChecklist}
        businessAssistantEnabled={settings.businessAssistantEnabled}
        followUpInbox={followUpInbox}
      />
      <ContactsList
        contacts={sortedContacts}
        keyExtractor={contactKeyExtractor}
        renderItem={renderContact}
        refreshing={refreshing}
        onRefresh={onRefresh}
        loading={loadingContacts}
        onInvite={handleQuickShareInvite}
        onAddPress={() => setAddModalVisible(true)}
      />
      <AddContactModal
        visible={isAddModalVisible}
        onClose={() => setAddModalVisible(false)}
        email={newContactEmail}
        onEmailChange={setNewContactEmail}
        onSearch={handleSearchContact}
        searchResults={searchResults}
        onSelectResult={setNewContactEmail}
        onAdd={handleAddContact}
      />
      <ContactActionSheet
        contact={selectedContact}
        onClose={() => setSelectedContact(null)}
        onCall={handleStartCallWithContact}
        onDetail={handleOpenDetail}
        onReport={openReportModal}
        onBlock={handleBlockUser}
        onRemove={handleRemoveContact}
      />
      <ReportModal
        target={reportTarget}
        onClose={closeReportModal}
        category={reportCategory}
        onCategoryChange={setReportCategory}
        details={reportDetails}
        onDetailsChange={setReportDetails}
        submitting={submittingReport}
        onSubmit={submitReport}
      />
    </View>
  );
};

export default ContactsScreen;
