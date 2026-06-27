import { registerPushDevice } from "@/api/platform";
import { runtimeConfig } from "@/lib/runtime-config";

const deviceIdKey = "allcallall.webPushDeviceId";
const permissionDismissedKey = "allcallall.webPushDismissed";

type MessagingModule = typeof import("firebase/messaging");
type Messaging = import("firebase/messaging").Messaging;

export const isPushConfigured = () => Boolean(runtimeConfig.firebase?.apiKey && runtimeConfig.firebase.vapidKey);
export const getStoredPushDeviceId = () => Number(localStorage.getItem(deviceIdKey) || 0) || null;
export const clearStoredPushDeviceId = () => localStorage.removeItem(deviceIdKey);
export const markPushDismissed = () => localStorage.setItem(permissionDismissedKey, "1");
export const wasPushDismissed = () => localStorage.getItem(permissionDismissedKey) === "1";

async function loadMessaging(): Promise<{ messaging: Messaging; module: MessagingModule } | null> {
  if (!isPushConfigured()) return null;
  const [{ initializeApp, getApps }, messagingModule] = await Promise.all([import("firebase/app"), import("firebase/messaging")]);
  if (!await messagingModule.isSupported()) return null;
  const app = getApps()[0] ?? initializeApp(runtimeConfig.firebase!);
  return { messaging: messagingModule.getMessaging(app), module: messagingModule };
}

export async function registerBrowserPush() {
  if (!("Notification" in window)) throw new Error("当前浏览器不支持通知。");
  if (Notification.permission === "denied") throw new Error("浏览器已拒绝通知权限，请在站点设置中开启。");
  const permission = Notification.permission === "granted" ? "granted" : await Notification.requestPermission();
  if (permission !== "granted") {
    markPushDismissed();
    throw new Error("通知权限未开启。");
  }
  const loaded = await loadMessaging();
  if (!loaded) throw new Error("Firebase Web Push 未配置或当前浏览器不支持。");
  const registration = await navigator.serviceWorker.register("/firebase-messaging-sw.js");
  const token = await loaded.module.getToken(loaded.messaging, { vapidKey: runtimeConfig.firebase!.vapidKey, serviceWorkerRegistration: registration });
  if (!token) throw new Error("未能获取浏览器推送 token。");
  const device = await registerPushDevice({
    token,
    provider: "fcm",
    platform: "web",
    device_name: navigator.userAgent.slice(0, 120),
    app_version: import.meta.env.VITE_APP_VERSION || "web-dev",
  });
  localStorage.setItem(deviceIdKey, String(device.id));
  localStorage.removeItem(permissionDismissedKey);
  return device;
}

export async function deleteBrowserPushToken() {
  const loaded = await loadMessaging();
  if (!loaded) return;
  await loaded.module.deleteToken(loaded.messaging);
}

export async function listenForegroundPush(onPayload: (payload: { title: string; body: string; url?: string }) => void) {
  const loaded = await loadMessaging();
  if (!loaded) return () => {};
  return loaded.module.onMessage(loaded.messaging, (payload) => {
    onPayload({
      title: payload.notification?.title || payload.data?.title || "AllCallAll",
      body: payload.notification?.body || payload.data?.body || "",
      url: payload.data?.url,
    });
  });
}
