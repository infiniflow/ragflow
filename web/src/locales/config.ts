import { LanguageAbbreviation } from '@/constants/common';
import storage from '@/utils/authorization-util';
import dayjs from 'dayjs';
import i18n from 'i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { upperFirst } from 'lodash';
import { initReactI18next } from 'react-i18next';
import translation_en from './en';
import translation_zh from './zh';
import translation_zh_traditional from './zh-traditional';

//The language is based on the .ng file stored in the client's local storage.
// The language stored in the database is for agent template resources, as these resources reside on the server.
// When a user logs in from a different machine, the login page language is the language configured by VITE_DEFAULT_LANGUAGE_CODE.

const languageImports: Record<string, () => Promise<{ default: any }>> = {
  [LanguageAbbreviation.En]: () => import('./en'),
  [LanguageAbbreviation.Zh]: () => import('./zh'),
  [LanguageAbbreviation.ZhTraditional]: () => import('./zh-traditional'),
  [LanguageAbbreviation.Id]: () => import('./id'),
  [LanguageAbbreviation.Ja]: () => import('./ja'),
  [LanguageAbbreviation.Es]: () => import('./es'),
  [LanguageAbbreviation.Vi]: () => import('./vi'),
  [LanguageAbbreviation.Ru]: () => import('./ru'),
  [LanguageAbbreviation.PtBr]: () => import('./pt-br'),
  [LanguageAbbreviation.De]: () => import('./de'),
  [LanguageAbbreviation.Fr]: () => import('./fr'),
  [LanguageAbbreviation.It]: () => import('./it'),
  [LanguageAbbreviation.Bg]: () => import('./bg'),
  [LanguageAbbreviation.Ar]: () => import('./ar'),
  [LanguageAbbreviation.Tr]: () => import('./tr'),
  [LanguageAbbreviation.Ko]: () => import('./ko'),
  [LanguageAbbreviation.Az]: () => import('./az'),
  [LanguageAbbreviation.Uz]: () => import('./uz'),
};

const supportedLanguageCodes: Intl.UnicodeBCP47LocaleIdentifier[] =
  Object.keys(languageImports);

export const supportedLanguages = supportedLanguageCodes.map((code) => {
  const locale = new Intl.Locale(code);

  return {
    code,
    locale,
    displayName: upperFirst(
      new Intl.DisplayNames(locale, { type: 'language' }).of(code)!,
    ),
  };
});

export const DEFAULT_LANGUAGE_CODE =
  import.meta.env.VITE_DEFAULT_LANGUAGE_CODE || LanguageAbbreviation.En;

// Enterprise customization overlays: the enterprise repository carries real
// implementations of these same-path files (`web/src/ee/locales/*-private.ts`);
// upstream ships them as empty objects so shared imports resolve in both
// trees. Enterprise content must live only in the ee/ files, never here.
import merge from 'lodash/merge';
import translation_en_private from '@/ee/locales/en-private';
import translation_zh_private from '@/ee/locales/zh-private';
import translation_zh_traditional_private from '@/ee/locales/zh-traditional-private';

const resources = {
  [LanguageAbbreviation.En]: merge(translation_en, translation_en_private),
  [LanguageAbbreviation.Zh]: merge(translation_zh, translation_zh_private),
  [LanguageAbbreviation.ZhTraditional]: merge(
    translation_zh_traditional,
    translation_zh_traditional_private,
  ),
};

const updateDocumentLocale = (lng: string) => {
  document.documentElement.lang = lng;
  document.documentElement.dir = 'ltr';
  dayjs.locale(lng === 'zh' ? 'zh-cn' : lng);
};

i18n
  .use(initReactI18next)
  .use(LanguageDetector)
  .init({
    detection: {
      lookupLocalStorage: 'lng',
      order: ['localStorage'],
      caches: [],
    },
    supportedLngs: supportedLanguageCodes,
    resources,
    fallbackLng: DEFAULT_LANGUAGE_CODE,
    interpolation: {
      escapeValue: false,
    },
  });

export const loadLanguageAsync = async (lng: string): Promise<void> => {
  const normalizedLng = lng;

  if (i18n.hasResourceBundle(normalizedLng, 'translation')) {
    return;
  }

  const importFn = languageImports[normalizedLng];
  if (!importFn) {
    console.warn(`Language ${lng} is not supported for lazy loading`);
    return;
  }

  try {
    const module = await importFn();
    const translationData = module.default?.translation || module.default;
    i18n.addResourceBundle(normalizedLng, 'translation', translationData);
  } catch (error) {
    console.error(`Failed to load language ${lng}:`, error);
  }
};

export const changeLanguageAsync = async (
  lng: string,
  options: { persist?: boolean } = {},
): Promise<void> => {
  const { persist = true } = options;
  const normalizedLng = lng;

  if (
    normalizedLng !== LanguageAbbreviation.En &&
    !i18n.hasResourceBundle(normalizedLng, 'translation')
  ) {
    await loadLanguageAsync(normalizedLng);
  }

  if (persist) {
    storage.setLanguage(lng);
  }

  updateDocumentLocale(lng);

  await i18n.changeLanguage(normalizedLng);
};

export const initLanguage = async (): Promise<void> => {
  const currentLng = storage.getLanguage() || DEFAULT_LANGUAGE_CODE;

  await changeLanguageAsync(currentLng);
};

export default i18n;
