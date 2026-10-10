import { LanguageAbbreviation } from '@/constants/common';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { en } from './locales/en';
import { zh } from './locales/zh';
import { TranslationContext } from './translation-context';

export function TranslationProvider({ children }: { children: ReactNode }) {
  const { i18n } = useTranslation();

  return (
    <TranslationContext.Provider
      value={i18n.language === LanguageAbbreviation.Zh ? zh : en}
    >
      {children}
    </TranslationContext.Provider>
  );
}
