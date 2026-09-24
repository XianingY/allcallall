import React from "react";
import { Text, TouchableOpacity, View } from "react-native";

import PresenceBadge from "../../components/PresenceBadge";
import PrimaryButton from "../../components/PrimaryButton";
import { styles } from "./styles";
import type {
  ChecklistItem,
  ContactsNavigation,
  ContactsViewer,
  FollowUpInboxSummary,
  OrganizationLike,
  PresenceMap
} from "./types";

interface ContactsOverviewProps {
  navigation: ContactsNavigation;
  user?: ContactsViewer | null;
  currentOrganization?: OrganizationLike | null;
  logout: () => void;
  presence: PresenceMap;
  checklistDismissed: boolean;
  checklistItems: ChecklistItem[];
  onDismissChecklist: () => void;
  businessAssistantEnabled: boolean;
  followUpInbox: FollowUpInboxSummary;
}

/**
 * 联系人页的概览区：问候头、我的状态、协作入口、首日引导与 Follow-up Inbox。
 * 纯展示组件——所有跳转与动作通过 props 注入，自身不持有任何状态。
 */
export const ContactsOverview: React.FC<ContactsOverviewProps> = ({
  navigation,
  user,
  currentOrganization,
  logout,
  presence,
  checklistDismissed,
  checklistItems,
  onDismissChecklist,
  businessAssistantEnabled,
  followUpInbox
}) => (
  <>
    <View style={styles.header}>
      <View>
        <Text style={styles.greeting}>你好, {user?.display_name || ""}</Text>
        <Text style={styles.subtitle}>{user?.email}</Text>
        {currentOrganization ? (
          <Text style={styles.workspaceText}>工作区 / Workspace: {currentOrganization.name}</Text>
        ) : null}
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

    <View style={styles.workspaceActionsCard}>
      <Text style={styles.sectionTitle}>协作平台 / Collaboration</Text>
      <View style={styles.workspaceActions}>
        <PrimaryButton title="工作区" onPress={() => navigation.navigate("Organizations")} style={styles.workspaceButton} />
        <PrimaryButton title="Inbox" onPress={() => navigation.navigate("Conversations")} style={styles.workspaceButton} />
        <PrimaryButton title="会议" onPress={() => navigation.navigate("Rooms")} style={styles.workspaceButton} />
        <PrimaryButton title="更多" onPress={() => navigation.navigate("Deals")} style={styles.workspaceButton} />
        <PrimaryButton title="录音" onPress={() => navigation.navigate("Recordings")} style={styles.workspaceButton} />
      </View>
    </View>

    {!checklistDismissed ? (
      <View style={styles.onboardingCard}>
        <View style={styles.onboardingHeader}>
          <Text style={styles.sectionTitle}>首日引导 / Onboarding</Text>
          <TouchableOpacity onPress={() => void onDismissChecklist()}>
            <Text style={styles.dismissText}>隐藏</Text>
          </TouchableOpacity>
        </View>
        <Text style={styles.onboardingDescription}>
          先邀请一个业务联系人，再完成首次跨语言通话和第一次回拨。
        </Text>
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

    {businessAssistantEnabled ? (
      <TouchableOpacity style={styles.followupCard} onPress={() => navigation.navigate("FollowUps")}>
        <View style={styles.followupHeader}>
          <Text style={styles.sectionTitle}>Follow-up Inbox</Text>
          <Text style={styles.followupLink}>查看全部</Text>
        </View>
        <Text style={styles.followupSummary}>
          逾期 {followUpInbox.overdue} 项 · 今日 {followUpInbox.today} 项
        </Text>
        {followUpInbox.topItems.length > 0 ? followUpInbox.topItems.map((item) => (
          <View key={item.task.id} style={styles.followupRow}>
            <Text style={styles.followupPeer}>{item.peer?.display_name || item.peer?.email || item.task.title}</Text>
            <Text style={styles.followupMeta}>{item.task.title}</Text>
          </View>
        )) : (
          <Text style={styles.followupEmpty}>完成第一通通话后，回访任务会出现在这里。</Text>
        )}
      </TouchableOpacity>
    ) : null}
  </>
);
