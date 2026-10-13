import { expect, it } from '@jest/globals';
import { act, render, screen } from '@testing-library/react';
import { createInstance } from 'i18next';
import { I18nextProvider, initReactI18next } from 'react-i18next';
import { useTranslation } from '../hooks/use-translation';
import { en } from './locales/en';
import { zh } from './locales/zh';
import { TranslationProvider } from './translation-provider';

function BuilderLabel() {
  const translation = useTranslation();
  return <span>{translation.fieldAddNewButton}</span>;
}

it('updates schema editor labels when switching between Chinese and English', async () => {
  const i18n = createInstance();
  await i18n.use(initReactI18next).init({
    lng: 'zh-Hans',
    fallbackLng: 'en',
    resources: { en: { translation: {} }, 'zh-Hans': { translation: {} } },
  });

  render(
    <I18nextProvider i18n={i18n}>
      <TranslationProvider>
        <BuilderLabel />
      </TranslationProvider>
    </I18nextProvider>,
  );

  expect(screen.getByText(zh.fieldAddNewButton)).toBeDefined();
  await act(async () => {
    await i18n.changeLanguage('en');
  });
  expect(screen.getByText(en.fieldAddNewButton)).toBeDefined();
  await act(async () => {
    await i18n.changeLanguage('zh-Hans');
  });
  expect(screen.getByText(zh.fieldAddNewButton)).toBeDefined();
  await act(async () => {
    await i18n.changeLanguage('fr');
  });
  expect(screen.getByText(en.fieldAddNewButton)).toBeDefined();
});

it('preserves every schema builder placeholder and technical example', () => {
  const placeholders = (value: string) =>
    [...value.matchAll(/\{\w+\}/g)].map((match) => match[0]).sort();
  expect(Object.keys(zh).sort()).toEqual(Object.keys(en).sort());
  for (const key of Object.keys(en) as Array<keyof typeof en>) {
    expect(placeholders(zh[key])).toEqual(placeholders(en[key]));
  }
  expect(zh.visualizerDownloadFileName).toBe(en.visualizerDownloadFileName);
});
