import { useCallback, useMemo, useState } from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { useFocusEffect } from "@react-navigation/native";

import { fetchCallHistory } from "../../api/commercial";
import { FOLLOW_UP_CALLS_STORAGE_KEY } from "../../constants/invitations";
import {
  FIRST_CALL_STARTED_STORAGE_KEY,
  FIRST_TRANSLATION_ENABLED_STORAGE_KEY,
  ONBOARDING_DISMISSED_STORAGE_KEY
} from "../../constants/onboarding";
import type { ChecklistItem } from "./types";

/**
 * 首日引导清单的状态与计算（#24 拆分时从 ContactsScreen 抽出）。
 * hasCallHistory 同时会被下拉刷新更新，因此把 setHasCallHistory 一并暴露出去。
 */
export const useOnboardingChecklist = (
  token: string | null,
  contactCount: number
) => {
  const [checklistDismissed, setChecklistDismissed] = useState(false);
  const [hasCallHistory, setHasCallHistory] = useState(false);
  const [hasStartedFirstCall, setHasStartedFirstCall] = useState(false);
  const [, setHasEnabledTranslation] = useState(false);
  const [followUpCallIds, setFollowUpCallIds] = useState<string[]>([]);
  const [firstBusinessContactTracked, setFirstBusinessContactTracked] =
    useState(false);

  const loadOnboardingState = useCallback(async () => {
    try {
      const [dismissed, firstCall, firstTranslation, followUps] =
        await Promise.all([
          AsyncStorage.getItem(ONBOARDING_DISMISSED_STORAGE_KEY),
          AsyncStorage.getItem(FIRST_CALL_STARTED_STORAGE_KEY),
          AsyncStorage.getItem(FIRST_TRANSLATION_ENABLED_STORAGE_KEY),
          AsyncStorage.getItem(FOLLOW_UP_CALLS_STORAGE_KEY)
        ]);
      const parsedFollowUps = followUps
        ? (JSON.parse(followUps) as string[])
        : [];
      setChecklistDismissed(dismissed === "dismissed");
      setHasStartedFirstCall(firstCall === "true");
      setHasEnabledTranslation(firstTranslation === "true");
      setFollowUpCallIds(parsedFollowUps);
      setFirstBusinessContactTracked(parsedFollowUps.length > 0);
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

  const dismissChecklist = useCallback(async () => {
    setChecklistDismissed(true);
    try {
      await AsyncStorage.setItem(ONBOARDING_DISMISSED_STORAGE_KEY, "dismissed");
    } catch {
      // Ignore onboarding storage failures.
    }
  }, []);

  const markFirstBusinessContactTracked = useCallback(() => {
    setFirstBusinessContactTracked(true);
  }, []);

  const checklistItems = useMemo<ChecklistItem[]>(() => {
    const hasContacts = contactCount > 0;
    return [
      {
        key: "invite",
        label: "邀请第一个业务联系人",
        done: hasContacts || firstBusinessContactTracked
      },
      {
        key: "call",
        label: "完成第一通跨语言通话",
        done: hasStartedFirstCall || hasCallHistory
      },
      {
        key: "followup",
        label: "完成第一次回拨 / 重复通话",
        done: followUpCallIds.length > 0 || hasCallHistory
      }
    ];
  }, [
    contactCount,
    firstBusinessContactTracked,
    followUpCallIds.length,
    hasCallHistory,
    hasStartedFirstCall
  ]);

  return {
    checklistDismissed,
    hasCallHistory,
    setHasCallHistory,
    dismissChecklist,
    markFirstBusinessContactTracked,
    checklistItems
  };
};
