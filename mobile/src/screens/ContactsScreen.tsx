import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  View,
  Text,
  StyleSheet,
  FlatList,
  RefreshControl,
  Alert,
  Modal,
  TouchableOpacity
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { useAuthContext } from "../context/AuthContext";
import {
  listContacts,
  addContact,
  removeContact,
  fetchPresence,
  User,
  PresenceRecord
} from "../api/users";
import ContactListItem from "../components/ContactListItem";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import PresenceBadge from "../components/PresenceBadge";
import CallOverlay from "../components/CallOverlay";
import { useSignaling } from "../context/SignalingContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "Contacts">;

const ContactsScreen: React.FC<Props> = ({ navigation }) => {
  const { user, token, logout } = useAuthContext();
  const { startCall, connectionReady } = useSignaling();

  const [contacts, setContacts] = useState<User[]>([]);
  const [presence, setPresence] = useState<Record<string, PresenceRecord>>({});
  const [loadingContacts, setLoadingContacts] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [isAddModalVisible, setAddModalVisible] = useState(false);
  const [newContactEmail, setNewContactEmail] = useState("");

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
      Alert.alert("拉取联系人失败", "无法加载联系人列表，请重试。");
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
    setRefreshing(false);
  }, [loadContacts, loadPresence]);

  const handleAddContact = useCallback(async () => {
    if (!token) {
      return;
    }
    const target = newContactEmail.trim().toLowerCase();
    if (!target) {
      return;
    }

    try {
      await addContact(token, target);
      setNewContactEmail("");
      setAddModalVisible(false);
      await loadContacts();
      await loadPresence();
      Alert.alert("联系人已添加", `${target} 已加入联系人。`);
    } catch (error) {
      console.error(error);
      Alert.alert("添加失败", "无法添加联系人，可能已存在或输入有误。");
    }
  }, [loadContacts, loadPresence, newContactEmail, token]);

  const handleRemoveContact = useCallback(
    (contact: User) => {
      Alert.alert(
        "删除联系人",
        `确定删除 ${contact.display_name || contact.email} 吗？`,
        [
          { text: "取消", style: "cancel" },
          {
            text: "删除",
            style: "destructive",
            onPress: async () => {
              if (!token) return;
              try {
                await removeContact(token, contact.id);
                await loadContacts();
                await loadPresence();
              } catch (error) {
                console.error(error);
                Alert.alert("删除失败", "请稍后再试。");
              }
            }
          }
        ]
      );
    },
    [loadContacts, loadPresence, token]
  );

  const handleStartCall = useCallback(
    (email: string) => {
      if (!connectionReady) {
        Alert.alert("正在重新连接", "信令服务暂时不可用，请稍后再试。");
        return;
      }
      startCall(email);
    },
    [connectionReady, startCall]
  );

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
            style={styles.changePasswordButton}
            onPress={() => navigation.navigate("ChangePassword")}
          >
            <Text style={styles.changePasswordText}>改密码</Text>
          </TouchableOpacity>
          <TouchableOpacity style={styles.logoutButton} onPress={logout}>
            <Text style={styles.logoutText}>退出登录</Text>
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
            onRemove={handleRemoveContact}
          />
        )}
        contentContainerStyle={styles.listContent}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={onRefresh} />
        }
        ListEmptyComponent={
          !loadingContacts ? (
            <Text style={styles.emptyText}>
              还没有联系人，点击“添加联系人”开始吧。
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
            <Text style={styles.modalTitle}>添加联系人</Text>
            <TextField
              label="邮箱 / Email"
              autoCapitalize="none"
              keyboardType="email-address"
              value={newContactEmail}
              onChangeText={setNewContactEmail}
            />
            <PrimaryButton title="添加" onPress={handleAddContact} />
            <PrimaryButton
              title="取消"
              onPress={() => setAddModalVisible(false)}
              style={styles.modalCancel}
            />
          </View>
        </View>
      </Modal>

      <CallOverlay />
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
  }
});

export default ContactsScreen;
