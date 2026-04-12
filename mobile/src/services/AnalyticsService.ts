export type AnalyticsEventName =
  | "signup_completed"
  | "first_contact_added"
  | "call_started"
  | "translation_started"
  | "paywall_viewed"
  | "purchase_completed";

export type AnalyticsEventParams = Record<string, string | number | boolean | null | undefined>;

class AnalyticsService {
  track(eventName: AnalyticsEventName, params?: AnalyticsEventParams) {
    if (__DEV__) {
      console.info("[AnalyticsService] track", eventName, params ?? {});
    }
  }
}

export default new AnalyticsService();
