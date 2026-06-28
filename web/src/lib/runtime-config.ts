const trimTrailingSlash = (value: string) => value.replace(/\/+$/, "");

const fromWindow = () => (typeof window === "undefined" ? undefined : window.__ALLCALLALL_CONFIG__);

const runtime = fromWindow();

const apiBaseUrl = trimTrailingSlash(
  runtime?.apiBaseUrl || import.meta.env.VITE_API_BASE_URL || "/api/v1",
);

const deriveWebSocketBaseUrl = () => {
  const explicit = runtime?.wsBaseUrl || import.meta.env.VITE_WS_BASE_URL;
  if (explicit) return trimTrailingSlash(explicit);

  try {
    const url = new URL(apiBaseUrl);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    return trimTrailingSlash(url.toString());
  } catch {
    const path = apiBaseUrl.startsWith("/") ? apiBaseUrl : `/${apiBaseUrl}`;
    if (typeof window === "undefined") return path;
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${window.location.host}${path}`;
  }
};

export const runtimeConfig = {
  apiBaseUrl,
  wsBaseUrl: deriveWebSocketBaseUrl(),
  firebase: runtime?.firebase,
  revenueCatPublicApiKey: runtime?.revenueCatPublicApiKey || import.meta.env.VITE_REVENUECAT_PUBLIC_API_KEY,
};
