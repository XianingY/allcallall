import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import en from "@/i18n/locales/en.json";
import zh from "@/i18n/locales/zh.json";

// Translations live in src/i18n/locales so both languages stay column-by-column
// reviewable; every key added to zh.json must also exist in en.json.
const resources = {
  zh: { translation: zh },
  en: { translation: en },
};

const storedLanguage = localStorage.getItem("allcallall.language");
void i18n.use(initReactI18next).init({
  resources,
  lng: storedLanguage === "en" ? "en" : "zh",
  fallbackLng: "zh",
  interpolation: { escapeValue: false },
});

export default i18n;

// SupportedLanguages drives the language switcher in Settings.
export const supportedLanguages = ["zh", "en"] as const;
