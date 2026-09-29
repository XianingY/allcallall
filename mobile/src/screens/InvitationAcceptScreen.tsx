import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Linking,
  StyleSheet,
  Text,
  View
} from "react-native";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import { useFocusEffect } from "@react-navigation/native";

import PrimaryButton from "../components/PrimaryButton";
import { RootStackParamList } from "../navigation/AppNavigator";
import {
  acceptInvitation,
  acceptOrganizationInvite,
  fetchInvitation,
  fetchOrganizationInvite,
  Invitation,
  OrganizationInvite
} from "../api/users";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import AnalyticsService from "../services/AnalyticsService";
import { PENDING_INVITATION_CODE_STORAGE_KEY } from "../constants/invitations";
import { parseInvitationCodeFromURL } from "../utils/invitations";

type Props = NativeStackScreenProps<RootStackParamList, "InvitationAccept">;

const InvitationAcceptScreen: React.FC<Props> = ({ navigation, route }) => {
  const { token, user } = useAuthContext();
  const { refreshOrganizations } = useOrganization();
  const [invitation, setInvitation] = useState<Invitation | null>(null);
  const [loading, setLoading] = useState(false);
  const [accepting, setAccepting] = useState(false);
  const [resolvedCode, setResolvedCode] = useState<string | null>(route.params?.code ?? null);
  // Organization invites are a separate table and endpoint. Trying only the
  // contact invitation endpoints made every organization invite look invalid
  // here, so a mobile user could not join an organization at all.
  const [orgInvite, setOrgInvite] = useState<OrganizationInvite | null>(null);

  useEffect(() => {
    const loadURL = async () => {
      if (resolvedCode) {
        return;
      }
      const initialURL = await Linking.getInitialURL();
      const parsed = parseInvitationCodeFromURL(initialURL);
      if (parsed) {
        setResolvedCode(parsed);
      }
    };
    void loadURL();
  }, [resolvedCode]);

  useFocusEffect(
    React.useCallback(() => {
      const subscription = Linking.addEventListener("url", ({ url }) => {
        const parsed = parseInvitationCodeFromURL(url);
        if (parsed) {
          setResolvedCode(parsed);
        }
      });
      return () => subscription.remove();
    }, [])
  );

  useEffect(() => {
    const loadInvitation = async () => {
      if (!resolvedCode) {
        return;
      }
      try {
        setLoading(true);
        // Organization invites need a token to look up; contact invitations
        // are readable anonymously. Try the organization one first when signed
        // in, then fall back.
        if (token) {
          try {
            const invite = await fetchOrganizationInvite(token, resolvedCode);
            setOrgInvite(invite);
            AnalyticsService.track("invite_opened", { code: resolvedCode, kind: "organization" });
            return;
          } catch (orgError) {
            console.warn("[InvitationAcceptScreen] Not an organization invite, trying contact invite:", orgError);
          }
        }
        const data = await fetchInvitation(resolvedCode);
        setInvitation(data);
        AnalyticsService.track("invite_opened", { code: resolvedCode, kind: "contact" });
      } catch (error) {
        console.error("[InvitationAcceptScreen] Failed to load invitation:", error);
        Alert.alert("邀请无效", "当前邀请不存在或已失效。");
      } finally {
        setLoading(false);
      }
    };
    void loadInvitation();
  }, [resolvedCode, token]);

  const statusText = useMemo(() => {
    if (orgInvite) {
      return `组织邀请 · 角色 ${orgInvite.role}\n目标邮箱：${orgInvite.target_email}`;
    }
    if (!invitation) {
      return "";
    }
    if (invitation.status === "accepted") {
      return "该邀请已被接受。";
    }
    if (invitation.status === "expired") {
      return "该邀请已过期。";
    }
    return `目标邮箱：${invitation.target_email}`;
  }, [invitation, orgInvite]);

  const handleAccept = async () => {
    if (!resolvedCode) {
      return;
    }
    if (!token || !user) {
      await AsyncStorage.setItem(PENDING_INVITATION_CODE_STORAGE_KEY, resolvedCode);
      navigation.navigate("Login");
      return;
    }
    try {
      setAccepting(true);
      if (orgInvite) {
        await acceptOrganizationInvite(token, resolvedCode);
        AnalyticsService.track("invite_accepted", { code: resolvedCode, kind: "organization" });
        await refreshOrganizations();
        Alert.alert("已加入组织", "该组织已出现在你的工作空间列表中。", [
          { text: "继续", onPress: () => navigation.navigate("Organizations") }
        ]);
        return;
      }
      await acceptInvitation(token, resolvedCode);
      AnalyticsService.track("invite_accepted", { code: resolvedCode, kind: "contact" });
      Alert.alert("邀请已接受", "联系人已自动添加到你的联系人列表。", [
        {
          text: "继续",
          onPress: () => navigation.navigate("Contacts")
        }
      ]);
    } catch (error) {
      console.error("[InvitationAcceptScreen] Failed to accept invitation:", error);
      Alert.alert("接受失败", "当前无法接受邀请，请确认邮箱匹配且邀请未过期。");
    } finally {
      setAccepting(false);
    }
  };

  return (
    <View style={styles.container}>
      <View style={styles.card}>
        <Text style={styles.title}>接受业务通话邀请</Text>
        {loading ? <Text style={styles.subtitle}>加载邀请中...</Text> : null}
        {invitation ? (
          <>
            <Text style={styles.inviter}>
              {invitation.inviter_display_name || invitation.inviter_email} 邀请你加入 AllCallAll
            </Text>
            <Text style={styles.subtitle}>{statusText}</Text>
            <Text style={styles.detail}>
              默认翻译: {invitation.default_source_lang || "auto"} → {invitation.default_target_lang || "auto"}
            </Text>
            {invitation.note ? <Text style={styles.note}>备注: {invitation.note}</Text> : null}
          </>
        ) : (
          <Text style={styles.subtitle}>请输入有效邀请链接后再继续。</Text>
        )}
        <PrimaryButton
          title={
            token
              ? accepting
                ? "接受中..."
                : "接受邀请并自动建联系人"
              : "登录或注册后接受邀请"
          }
          onPress={() => void handleAccept()}
          disabled={!resolvedCode || accepting}
        />
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 20,
    justifyContent: "center"
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 22
  },
  title: {
    fontSize: 24,
    fontWeight: "800",
    color: "#0f172a"
  },
  inviter: {
    marginTop: 14,
    fontSize: 18,
    fontWeight: "700",
    color: "#111827"
  },
  subtitle: {
    marginTop: 10,
    color: "#475569",
    lineHeight: 20
  },
  detail: {
    marginTop: 12,
    color: "#0f172a",
    fontWeight: "600"
  },
  note: {
    marginTop: 10,
    color: "#475569"
  }
});

export default InvitationAcceptScreen;
