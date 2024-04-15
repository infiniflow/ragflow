import i18n from '@/locales/config';
import { App, ConfigProvider, ConfigProviderProps } from 'antd';
import enUS from 'antd/locale/en_US';
import zhCN from 'antd/locale/zh_CN';
import zh_HK from 'antd/locale/zh_HK';
import React, { ReactNode, useEffect, useState } from 'react';
import storage from './utils/authorizationUtil';

const AntLanguageMap = {
  en: enUS,
  zh: zhCN,
  'zh-TRADITIONAL': zh_HK,
};

type Locale = ConfigProviderProps['locale'];

const RootProvider = ({ children }: React.PropsWithChildren) => {
  const getLocale = (lng: string) =>
    AntLanguageMap[lng as keyof typeof AntLanguageMap] ?? enUS;

  const [locale, setLocal] = useState<Locale>(getLocale(storage.getLanguage()));

  i18n.on('languageChanged', function (lng: string) {
    storage.setLanguage(lng);
    setLocal(getLocale(lng));
  });

  useEffect(() => {
    // Because the language is saved in the backend, a token is required to obtain the api. However, the login page cannot obtain the language through the getUserInfo api, so the language needs to be saved in localstorage.
    const lng = storage.getLanguage();
    if (lng) {
      i18n.changeLanguage(lng);
    }
  }, []);

  return (
    <ConfigProvider
      theme={{
        token: {
          fontFamily: 'Inter',
        },
      }}
      locale={locale}
    >
      <App> {children}</App>
    </ConfigProvider>
  );
};

export function rootContainer(container: ReactNode) {
  return <RootProvider>{container}</RootProvider>;
}
