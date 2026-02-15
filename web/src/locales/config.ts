import i18n from 'i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { initReactI18next } from 'react-i18next';

import { LanguageAbbreviation } from '@/constants/common';
import { createTranslationTable, flattenObject } from './until';

import translation_en from './en';

const languageImports: Record<string, () => Promise<{ default: any }>> = {
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
};

const enFlattened = flattenObject(translation_en);

export const translationTable = createTranslationTable(
  [enFlattened],
  ['English'],
);

const resources = {
  [LanguageAbbreviation.En]: translation_en,
};

i18n
  .use(initReactI18next)
  .use(LanguageDetector)
  .init({
    detection: {
      lookupLocalStorage: 'lng',
    },
    supportedLngs: Object.values(LanguageAbbreviation),
    resources,
    fallbackLng: 'en',
    interpolation: {
      escapeValue: false,
    },
  });

export const loadLanguageAsync = async (lng: string): Promise<void> => {
  if (i18n.hasResourceBundle(lng, 'translation')) {
    return;
  }

  const importFn = languageImports[lng];
  if (!importFn) {
    console.warn(`Language ${lng} is not supported for lazy loading`);
    return;
  }

  try {
    const module = await importFn();
    const translationData = module.default?.translation || module.default;
    i18n.addResourceBundle(lng, 'translation', translationData);

    const flattened = flattenObject({ translation: translationData });
    translationTable.push(flattened);
  } catch (error) {
    console.error(`Failed to load language ${lng}:`, error);
  }
};

export const changeLanguageAsync = async (lng: string): Promise<void> => {
  if (lng !== 'en' && !i18n.hasResourceBundle(lng, 'translation')) {
    await loadLanguageAsync(lng);
  }
  await i18n.changeLanguage(lng);
};

export const initLanguage = async (): Promise<void> => {
  const currentLng = i18n.language || localStorage.getItem('lng') || 'en';

  if (currentLng !== 'en' && languageImports[currentLng]) {
    await loadLanguageAsync(currentLng);
    await i18n.changeLanguage(currentLng);
  }
};

export default i18n;
