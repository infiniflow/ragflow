import { useContext } from 'react';
import { en } from '../i18n/locales/en';
import { TranslationContext } from '../i18n/translation-context';

export function useTranslation() {
  const translation = useContext(TranslationContext);
  return translation ?? en;
}

export function formatTranslation(
  template: string,
  values: Record<string, string | number>,
) {
  return template.replace(/\{(\w+)\}/g, (_, key) => {
    const value = values[key];
    return value !== undefined ? String(value) : `{${key}}`;
  });
}
