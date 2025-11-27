import React from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import { ActivityIndicator, View } from "react-native";

import { useAuthContext } from "../context/AuthContext";
import LoginScreen from "../screens/LoginScreen";
import RegisterScreen from "../screens/RegisterScreen";
import EmailVerificationScreen from "../screens/EmailVerificationScreen";
import ContactsScreen from "../screens/ContactsScreen";
import ChangePasswordScreen from "../screens/ChangePasswordScreen";

export type RootStackParamList = {
  Login: undefined;
  Register: { email?: string };
  EmailVerification: { email?: string; onVerified?: () => void };
  Contacts: undefined;
  ChangePassword: undefined;
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
            name="ChangePassword"
            component={ChangePasswordScreen}
            options={{ title: "修改密码 / Change Password" }}
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
            name="EmailVerification"
            component={EmailVerificationScreen}
            options={{ title: "邮箱验证 / Email Verification" }}
          />
        </>
      )}
    </Stack.Navigator>
  );
};

export default AppNavigator;
