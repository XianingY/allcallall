export type AnalyticsEventName =
  | "signup_completed"
  | "first_contact_added"
  | "first_business_contact_added"
  | "call_started"
  | "first_call_completed"
  | "second_call_completed"
  | "missed_call_callback_started"
  | "meeting_join_failed"
  | "meeting_reconnect_started"
  | "meeting_reconnect_succeeded"
  | "meeting_reconnect_failed"
  | "meeting_remote_stream_lost"
  | "translation_started"
  | "paywall_viewed"
  | "purchase_completed"
  | "invite_created"
  | "invite_opened"
  | "invite_accepted"
  | "followup_generated"
  | "followup_viewed"
  | "followup_task_created"
  | "followup_task_completed"
  | "draft_copied";

export type AnalyticsEventParams = Record<string, string | number | boolean | null | undefined>;

class AnalyticsService {
  track(eventName: AnalyticsEventName, params?: AnalyticsEventParams) {
    if (__DEV__) {
      console.info("[AnalyticsService] track", eventName, params ?? {});
    }
  }
}

export default new AnalyticsService();
