import i18n from "i18next";
import { initReactI18next } from "react-i18next";

const resources = {
  zh: { translation: {
    "brand.tagline": "会议协作与 Agent 工作台",
    "nav.inbox": "协作 Inbox", "nav.meetings": "会议", "nav.agent": "Agent Lab",
    "nav.knowledge": "知识库", "nav.contacts": "联系人", "nav.deals": "商机",
    "nav.recordings": "录音转写", "nav.followups": "跟进", "nav.settings": "设置",
    "common.loading": "正在加载", "common.retry": "重试",
  } },
  en: { translation: {
    "brand.tagline": "Meeting collaboration and Agent workspace",
    "nav.inbox": "Inbox", "nav.meetings": "Meetings", "nav.agent": "Agent Lab",
    "nav.knowledge": "Knowledge", "nav.contacts": "Contacts", "nav.deals": "Deals",
    "nav.recordings": "Transcripts", "nav.followups": "Follow-ups", "nav.settings": "Settings",
    "common.loading": "Loading", "common.retry": "Retry",
  } },
};

const storedLanguage = localStorage.getItem("allcallall.language");
void i18n.use(initReactI18next).init({
  resources,
  lng: storedLanguage === "en" ? "en" : "zh",
  fallbackLng: "zh",
  interpolation: { escapeValue: false },
});

export default i18n;
