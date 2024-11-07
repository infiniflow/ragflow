import i18n from 'i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { initReactI18next } from 'react-i18next';

import translation_en from './en';
import translation_es from './es';
import translation_id from './id';
import translation_zh from './zh';
import translation_zh_traditional from './zh-traditional';
const resources = {
  en: translation_en,
  zh: translation_zh,
  'zh-TRADITIONAL': translation_zh_traditional,
  id: translation_id,
  es: translation_es,
};

i18n
  .use(initReactI18next)
  .use(LanguageDetector)
  .init({
    detection: {
      lookupLocalStorage: 'lng',
    },
    supportedLngs: ['en', 'zh', 'zh-TRADITIONAL', 'id', 'es'],
    resources,
    fallbackLng: 'en',
    interpolation: {
      escapeValue: false,
    },
  });

export default i18n;
