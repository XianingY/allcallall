import React from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import { ActivityIndicator, View } from "react-native";

import { useAuthContext } from "../context/AuthContext";
import LoginScreen from "../screens/LoginScreen";
import RegisterScreen from "../screens/RegisterScreen";
import EmailVerificationScreen from "../screens/EmailVerificationScreen";
import ContactsScreen from "../screens/ContactsScreen";
import ChangePasswordScreen from "../screens/ChangePasswordScreen";
import SettingsScreen from "../screens/SettingsScreen";
import ForgotPasswordScreen from "../screens/ForgotPasswordScreen";
import CallHistoryScreen from "../screens/CallHistoryScreen";
import SubscriptionScreen from "../screens/SubscriptionScreen";
import LegalScreen from "../screens/LegalScreen";
import BlockedUsersScreen from "../screens/BlockedUsersScreen";
import DeleteAccountScreen from "../screens/DeleteAccountScreen";
import ContactDetailScreen from "../screens/ContactDetailScreen";
import InvitationScreen from "../screens/InvitationScreen";
import InvitationAcceptScreen from "../screens/InvitationAcceptScreen";
import FollowUpsScreen from "../screens/FollowUpsScreen";
import type { User } from "../api/users";

export type RootStackParamList = {
  Login: undefined;
  Register: { email?: string };
  EmailVerification: { email?: string; returnToRegister?: boolean } | undefined;
  ForgotPassword: undefined;
  Contacts: undefined;
  CallHistory: undefined;
  ContactDetail: { contact: User };
  Invitation: undefined;
  InvitationAccept: { code?: string } | undefined;
  FollowUps: undefined;
  ChangePassword: undefined;
  Settings: undefined;
  Subscription: undefined;
  Legal: undefined;
  BlockedUsers: undefined;
  DeleteAccount: undefined;
};

const Stack = createNativeStackNavigator<RootStackParamList>();

const LoadingFallback = () => (
  <View
    style={{
      flex: 1,
      justifyContent: "center",
      alignItems: "center"
    }}
  >
    <ActivityIndicator size="large" />
  </View>
);

const AppNavigator: React.FC = () => {
  const { token, loading } = useAuthContext();

  if (loading) {
    return <LoadingFallback />;
  }

  return (
    <Stack.Navigator>
      {token ? (
        <>
          <Stack.Screen
            name="Contacts"
            component={ContactsScreen}
            options={{ headerShown: false }}
          />
          <Stack.Screen
            name="CallHistory"
            component={CallHistoryScreen}
            options={{ title: "最近通话 / Recent Calls" }}
          />
          <Stack.Screen
            name="ContactDetail"
            component={ContactDetailScreen}
            options={{ title: "联系人详情 / Contact Detail" }}
          />
          <Stack.Screen
            name="FollowUps"
            component={FollowUpsScreen}
            options={{ title: "跟进工作台 / Follow-ups" }}
          />
          <Stack.Screen
            name="Invitation"
            component={InvitationScreen}
            options={{ title: "邀请试用 / Invite Contact" }}
          />
          <Stack.Screen
            name="InvitationAccept"
            component={InvitationAcceptScreen}
            options={{ title: "接受邀请 / Accept Invitation" }}
          />
          <Stack.Screen
            name="ChangePassword"
            component={ChangePasswordScreen}
            options={{ title: "修改密码 / Change Password" }}
          />
          <Stack.Screen
            name="Settings"
            component={SettingsScreen}
            options={{ title: "设置 / Settings" }}
          />
          <Stack.Screen
            name="Subscription"
            component={SubscriptionScreen}
            options={{ title: "Premium / Subscription" }}
          />
          <Stack.Screen
            name="Legal"
            component={LegalScreen}
            options={{ title: "法律与合规 / Legal" }}
          />
          <Stack.Screen
            name="BlockedUsers"
            component={BlockedUsersScreen}
            options={{ title: "黑名单 / Blocked Users" }}
          />
          <Stack.Screen
            name="DeleteAccount"
            component={DeleteAccountScreen}
            options={{ title: "删除账号 / Delete Account" }}
          />
        </>
      ) : (
        <>
          <Stack.Screen
            name="Login"
            component={LoginScreen}
            options={{ title: "AllCallAll 登录 / Login" }}
          />
          <Stack.Screen
            name="Register"
            component={RegisterScreen}
            options={{ title: "AllCallAll 注册 / Register" }}
          />
          <Stack.Screen
            name="ForgotPassword"
            component={ForgotPasswordScreen}
            options={{ title: "重置密码 / Forgot Password" }}
          />
          <Stack.Screen
            name="EmailVerification"
            component={EmailVerificationScreen}
            options={{ title: "邮箱验证 / Email Verification" }}
          />
          <Stack.Screen
            name="InvitationAccept"
            component={InvitationAcceptScreen}
            options={{ title: "接受邀请 / Accept Invitation" }}
          />
        </>
      )}
    </Stack.Navigator>
  );
};

export default AppNavigator;
