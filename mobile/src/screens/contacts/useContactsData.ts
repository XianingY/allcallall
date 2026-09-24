import { useCallback, useEffect, useState } from "react";
import { Alert } from "react-native";

import { fetchPresence, listContacts, type User } from "../../api/users";
import type { PresenceMap } from "./types";

interface UseContactsDataArgs {
  token: string | null;
  userEmail?: string | null;
  onSessionExpired: () => void;
  isUnauthorizedError: (error: unknown) => boolean;
}

/**
 * 联系人列表与在线状态的加载（#24 拆分时从 ContactsScreen 抽出）。
 * 保留原有的三处 effect 语义：首次加载、10s 轮询 presence、contacts 变化后刷新 presence。
 */
export const useContactsData = ({
  token,
  userEmail,
  onSessionExpired,
  isUnauthorizedError
}: UseContactsDataArgs) => {
  const [contacts, setContacts] = useState<User[]>([]);
  const [presence, setPresence] = useState<PresenceMap>({});
  const [loadingContacts, setLoadingContacts] = useState(false);

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
      if (isUnauthorizedError(error)) {
        onSessionExpired();
        return;
      }
      Alert.alert(
        "拉取联系人失败 / Failed to load contacts",
        "无法加载联系人列表，请重试 / Please try again later."
      );
    } finally {
      setLoadingContacts(false);
    }
  }, [isUnauthorizedError, onSessionExpired, token]);

  const loadPresence = useCallback(async () => {
    if (!token) {
      return;
    }
    const emails = [userEmail, ...contacts.map((c) => c.email)].filter(
      Boolean
    ) as string[];

    if (!emails.length) {
      return;
    }

    try {
      const presenceList = await fetchPresence(token, emails);
      const map: PresenceMap = {};
      presenceList.forEach((record) => {
        map[record.email] = record;
      });
      setPresence(map);
    } catch (error) {
      if (isUnauthorizedError(error)) {
        onSessionExpired();
        return;
      }
      console.warn("presence load failed", error);
    }
  }, [contacts, isUnauthorizedError, onSessionExpired, token, userEmail]);

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

  return { contacts, presence, loadingContacts, loadContacts, loadPresence };
};
