import axios, { AxiosInstance } from "axios";

import { API_BASE_URL, REQUEST_TIMEOUT } from "../config";

let activeOrganizationId: number | null = null;

// Access token is held at module scope so a client built for one request can
// still pick up a token that was refreshed by another request. Callers may
// still pass an explicit token, which wins.
let accessToken: string | null = null;

type TokenRefresher = () => Promise<string | null>;

let tokenRefresher: TokenRefresher | null = null;
let refreshInFlight: Promise<string | null> | null = null;

// Requests already retried once after a refresh. A WeakSet so entries are
// collected with the config object instead of piling up per request.
const retriedAfterRefresh = new WeakSet<object>();

export const setActiveOrganizationHeader = (organizationId: number | null) => {
  activeOrganizationId = organizationId;
};

export const getActiveOrganizationHeader = () => activeOrganizationId;

export const setAccessToken = (token: string | null) => {
  accessToken = token;
};

export const getAccessToken = () => accessToken;

/**
 * Register how to mint a new access token. AuthContext owns the refresh call
 * and the secure-storage write, so it wires itself in here.
 *
 * Without this, a short access-token TTL simply logs the user out: expired
 * requests fail with 401 and nothing tries to renew the token.
 */
export const configureTokenRefresh = (refresher: TokenRefresher | null) => {
  tokenRefresher = refresher;
};

// Instances are cached rather than rebuilt per call. createApiClient is called
// at 100+ sites - once per request - and each call was constructing an axios
// instance plus two interceptors. Two slots is enough: one for the signed-in
// token, one for anonymous requests, which is all the app ever uses. Keyed by
// token, so a caller that passes an explicit token still gets an instance
// bound to that token.
let cachedToken: string | null = null;
let cachedInstance: AxiosInstance | null = null;
let anonymousInstance: AxiosInstance | null = null;

export const createApiClient = (token?: string): AxiosInstance => {
  if (!token) {
    if (!anonymousInstance) {
      anonymousInstance = buildClient(undefined);
    }
    return anonymousInstance;
  }
  if (cachedInstance && cachedToken === token) {
    return cachedInstance;
  }
  cachedInstance = buildClient(token);
  cachedToken = token;
  return cachedInstance;
};

const buildClient = (token?: string): AxiosInstance => {
  const instance = axios.create({
    baseURL: API_BASE_URL,
    timeout: REQUEST_TIMEOUT
  });

  instance.interceptors.request.use((config) => {
    const authToken = token ?? accessToken;
    if (authToken) {
      config.headers.Authorization = `Bearer ${authToken}`;
    }
    if (activeOrganizationId) {
      config.headers["X-Organization-ID"] = String(activeOrganizationId);
    }
    config.headers["Content-Type"] = "application/json";
    config.headers["Accept"] = "application/json";
    return config;
  });

  instance.interceptors.response.use(
    (response) => response,
    async (error: unknown) => {
      if (!axios.isAxiosError(error)) {
        return Promise.reject(error);
      }
      const config = error.config;
      const status = error.response?.status;

      // Only retry requests that actually carried a token, and only once.
      // Requests without a token (login, refresh itself) must not recurse.
      if (status !== 401 || !config || !tokenRefresher || retriedAfterRefresh.has(config)) {
        return Promise.reject(error);
      }
      retriedAfterRefresh.add(config);

      // Share one refresh across every request that failed at the same time,
      // otherwise a burst of 401s fires a burst of refresh calls.
      if (!refreshInFlight) {
        refreshInFlight = tokenRefresher()
          .catch(() => null)
          .finally(() => {
            refreshInFlight = null;
          });
      }
      const renewed = await refreshInFlight;
      if (!renewed) {
        return Promise.reject(error);
      }

      config.headers.Authorization = `Bearer ${renewed}`;
      return instance.request(config);
    }
  );

  return instance;
};
