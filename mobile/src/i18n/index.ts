import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { getLocales } from 'expo-localization';

import en from './locales/en.json';
import zh from './locales/zh.json';

const resources = {
  en: { translation: en },
  zh: { translation: zh },
};

// 获取系统语言，默认为中文
const deviceLanguage = getLocales()[0]?.languageCode ?? 'zh';
const initialLang = deviceLanguage.startsWith('zh') ? 'zh' : 'en';

i18n
  .use(initReactI18next)
  .init({
    resources,
    lng: initialLang,
    fallbackLng: 'en',
    interpolation: {
      escapeValue: false, // React 已经处理了 XSS
    },
  });

export default i18n;
