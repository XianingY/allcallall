/// <reference types="vite/client" />

interface AllCallAllRuntimeConfig {
  apiBaseUrl?: string;
  wsBaseUrl?: string;
  firebase?: {
    apiKey: string;
    authDomain: string;
    projectId: string;
    storageBucket?: string;
    messagingSenderId: string;
    appId: string;
    vapidKey: string;
  };
  revenueCatPublicApiKey?: string;
}

interface Window {
  __ALLCALLALL_CONFIG__?: AllCallAllRuntimeConfig;
}
