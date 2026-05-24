import { Platform } from "react-native";

export interface PushRemoteMessage {
  data?: Record<string, string>;
}

export interface PushAdapter {
  isSupported(): boolean;
  requestPermission(): Promise<boolean>;
  getToken(): Promise<string | null>;
  onMessage(listener: (message: PushRemoteMessage) => void): () => void;
  onNotificationOpenedApp(listener: (message: PushRemoteMessage) => void): () => void;
  setBackgroundMessageHandler(listener: (message: PushRemoteMessage) => Promise<void> | void): void;
  onTokenRefresh(listener: (token: string) => void): () => void;
  unregister(): Promise<void>;
  hasPermission(): Promise<boolean>;
  getProvider(): "fcm" | "apns" | "webpush";
}

const webAdapter: PushAdapter = {
  isSupported() {
    return false;
  },
  async requestPermission() {
    return false;
  },
  async getToken() {
    return null;
  },
  onMessage() {
    return () => undefined;
  },
  onNotificationOpenedApp() {
    return () => undefined;
  },
  setBackgroundMessageHandler() {
    return;
  },
  onTokenRefresh() {
    return () => undefined;
  },
  async unregister() {
    return;
  },
  async hasPermission() {
    return false;
  },
  getProvider() {
    return "webpush";
  },
};

const nativeAdapter: PushAdapter = {
  isSupported() {
    return true;
  },
  async requestPermission() {
    const messaging = require("@react-native-firebase/messaging").default;
    const authStatus = await messaging().requestPermission();
    return (
      authStatus === messaging.AuthorizationStatus.AUTHORIZED ||
      authStatus === messaging.AuthorizationStatus.PROVISIONAL
    );
  },
  async getToken() {
    const messaging = require("@react-native-firebase/messaging").default;
    return messaging().getToken();
  },
  onMessage(listener) {
    const messaging = require("@react-native-firebase/messaging").default;
    return messaging().onMessage(listener);
  },
  onNotificationOpenedApp(listener) {
    const messaging = require("@react-native-firebase/messaging").default;
    return messaging().onNotificationOpenedApp(listener);
  },
  setBackgroundMessageHandler(listener) {
    const messaging = require("@react-native-firebase/messaging").default;
    messaging().setBackgroundMessageHandler(listener);
  },
  onTokenRefresh(listener) {
    const messaging = require("@react-native-firebase/messaging").default;
    return messaging().onTokenRefresh(listener);
  },
  async unregister() {
    const messaging = require("@react-native-firebase/messaging").default;
    await messaging().unregisterDeviceForRemoteMessages();
  },
  async hasPermission() {
    const messaging = require("@react-native-firebase/messaging").default;
    const authStatus = await messaging().hasPermission();
    return (
      authStatus === messaging.AuthorizationStatus.AUTHORIZED ||
      authStatus === messaging.AuthorizationStatus.PROVISIONAL
    );
  },
  getProvider() {
    return "fcm";
  },
};

const pushAdapter: PushAdapter = Platform.OS === "web" ? webAdapter : nativeAdapter;

export default pushAdapter;
