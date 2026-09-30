import { runtimeConfig } from "@/lib/runtime-config";

export interface APIErrorBody {
  success?: false;
  code?: string;
  error?: string;
  request_id?: string;
}

export class APIError extends Error {
  /** Set by {@link markSessionExpired} when the refresh flow itself failed with 401. */
  sessionExpired = false;

  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId?: string,
  ) {
    super(message);
    this.name = "APIError";
  }
}

/** Tag an error as coming from the refresh-exhaustion path (refresh cookie dead). */
export const markSessionExpired = (error: unknown): void => {
  if (error instanceof APIError) error.sessionExpired = true;
};

/**
 * A 401 that survived a refresh attempt: the session itself is over, not just
 * this one request. Plain 401s (a single forbidden call) stay unmarked.
 */
export const isSessionExpiredError = (error: unknown): error is APIError =>
  error instanceof APIError && error.status === 401 && error.sessionExpired === true;

interface AuthPayload {
  access_token: string;
}

let accessToken: string | null = null;
let organizationId: number | null = null;
let refreshPromise: Promise<AuthPayload> | null = null;

export const setAccessToken = (token: string | null) => { accessToken = token; };
export const getAccessToken = () => accessToken;
export const setOrganizationId = (id: number | null) => { organizationId = id; };
export const getOrganizationId = () => organizationId;

const parseResponse = async <T>(response: Response): Promise<T> => {
  if (response.status === 204) return undefined as T;
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) return undefined as T;
  return response.json() as Promise<T>;
};

const toAPIError = async (response: Response) => {
  let body: APIErrorBody = {};
  try { body = await response.json() as APIErrorBody; } catch { /* no structured body */ }
  return new APIError(response.status, body.code ?? "HTTP_ERROR", body.error ?? response.statusText, body.request_id);
};

const refresh = async (): Promise<AuthPayload> => {
  if (!refreshPromise) {
    refreshPromise = fetch(`${runtimeConfig.apiBaseUrl}/auth/refresh`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json" },
    }).then(async (response) => {
      if (!response.ok) {
        // Refresh exhausted: tag the 401 so the query layer can tell a dead
        // session (end it) apart from an ordinary forbidden request (keep it).
        const error = await toAPIError(response);
        if (error.status === 401) markSessionExpired(error);
        throw error;
      }
      const payload = await parseResponse<AuthPayload>(response);
      setAccessToken(payload.access_token);
      return payload;
    }).finally(() => { refreshPromise = null; });
  }
  return refreshPromise;
};

interface RequestOptions {
  auth?: boolean;
  retry401?: boolean;
}

export async function apiRequest<T>(path: string, init: RequestInit = {}, options: RequestOptions = {}): Promise<T> {
  const auth = options.auth ?? true;
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  if (auth && accessToken) headers.set("Authorization", `Bearer ${accessToken}`);
  if (auth && organizationId) headers.set("X-Organization-ID", String(organizationId));

  const response = await fetch(`${runtimeConfig.apiBaseUrl}${path}`, { ...init, headers, credentials: "include" });
  if (response.status === 401 && auth && (options.retry401 ?? true)) {
    try {
      await refresh();
      return apiRequest<T>(path, init, { ...options, retry401: false });
    } catch (error) {
      setAccessToken(null);
      throw error;
    }
  }
  if (!response.ok) throw await toAPIError(response);
  return parseResponse<T>(response);
}

export const refreshAccessToken = refresh;

export async function apiDownload(path: string, retry401 = true): Promise<{ blob: Blob; fileName?: string }> {
  const headers = new Headers({ Accept: "*/*" });
  if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);
  if (organizationId) headers.set("X-Organization-ID", String(organizationId));
  const response = await fetch(`${runtimeConfig.apiBaseUrl}${path}`, { headers, credentials: "include" });
  if (response.status === 401 && retry401) { await refresh(); return apiDownload(path, false); }
  if (!response.ok) throw await toAPIError(response);
  const disposition = response.headers.get("content-disposition") ?? "";
  const match = disposition.match(/filename="?([^";]+)"?/i);
  return { blob: await response.blob(), fileName: match?.[1] };
}
