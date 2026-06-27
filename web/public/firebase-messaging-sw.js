/* global firebase, importScripts, self */
importScripts("/config.js");
importScripts("https://www.gstatic.com/firebasejs/10.13.2/firebase-app-compat.js");
importScripts("https://www.gstatic.com/firebasejs/10.13.2/firebase-messaging-compat.js");

const config = self.__ALLCALLALL_CONFIG__ || {};

if (config.firebase && config.firebase.apiKey) {
  firebase.initializeApp(config.firebase);
  const messaging = firebase.messaging();

  messaging.onBackgroundMessage((payload) => {
    const title = payload.notification?.title || payload.data?.title || "AllCallAll";
    const body = payload.notification?.body || payload.data?.body || "";
    const url = payload.data?.url || "/";
    self.registration.showNotification(title, {
      body,
      data: { url },
    });
  });
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = event.notification.data?.url || "/";
  event.waitUntil((async () => {
    const clientsList = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    for (const client of clientsList) {
      if ("focus" in client) {
        client.navigate(url);
        return client.focus();
      }
    }
    return self.clients.openWindow(url);
  })());
});
