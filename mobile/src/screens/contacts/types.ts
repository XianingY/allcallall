import type { NativeStackScreenProps } from "@react-navigation/native-stack";

import type { FollowUpListItem } from "../../api/commercial";
import type { PresenceRecord, User } from "../../api/users";
import type { RootStackParamList } from "../../navigation/AppNavigator";

export type ContactsScreenProps = NativeStackScreenProps<
  RootStackParamList,
  "Contacts"
>;

/** 供抽离子组件使用的导航对象类型，避免各模块重复声明。 */
export type ContactsNavigation = ContactsScreenProps["navigation"];

export type ReportCategory =
  | "spam"
  | "harassment"
  | "impersonation"
  | "fraud"
  | "sexual_content"
  | "other";

export interface ReportCategoryOption {
  value: ReportCategory;
  label: string;
  description: string;
}

export interface ChecklistItem {
  key: string;
  label: string;
  done: boolean;
}

export interface FollowUpInboxSummary {
  overdue: number;
  today: number;
  topItems: FollowUpListItem[];
}

/** 只声明子组件真正读取的字段，避免与 AuthContext 的完整 User 强耦合。 */
export interface ContactsViewer {
  display_name?: string | null;
  email?: string | null;
}

export interface OrganizationLike {
  id: number;
  name: string;
}

export type PresenceMap = Record<string, PresenceRecord>;

export type ContactKeyExtractor = (item: User) => string;

export type ContactRenderItem = (info: { item: User }) => React.ReactElement;
