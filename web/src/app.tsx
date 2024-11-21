import i18n from '@/locales/config';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';
import { App, ConfigProvider, ConfigProviderProps } from 'antd';
import enUS from 'antd/locale/en_US';
import vi_VN from 'antd/locale/vi_VN';
import zhCN from 'antd/locale/zh_CN';
import zh_HK from 'antd/locale/zh_HK';
import dayjs from 'dayjs';
import advancedFormat from 'dayjs/plugin/advancedFormat';
import customParseFormat from 'dayjs/plugin/customParseFormat';
import localeData from 'dayjs/plugin/localeData';
import weekOfYear from 'dayjs/plugin/weekOfYear';
import weekYear from 'dayjs/plugin/weekYear';
import weekday from 'dayjs/plugin/weekday';
import React, { ReactNode, useEffect, useState } from 'react';
import { ThemeProvider } from './components/theme-provider';
import storage from './utils/authorization-util';

dayjs.extend(customParseFormat);
dayjs.extend(advancedFormat);
dayjs.extend(weekday);
dayjs.extend(localeData);
dayjs.extend(weekOfYear);
dayjs.extend(weekYear);

const AntLanguageMap = {
  en: enUS,
  zh: zhCN,
  'zh-TRADITIONAL': zh_HK,
  vi: vi_VN,
};

const queryClient = new QueryClient();

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
    <QueryClientProvider client={queryClient}>
      <ThemeProvider defaultTheme="light" storageKey="ragflow-ui-theme">
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
        <ReactQueryDevtools buttonPosition={'top-left'} />
      </ThemeProvider>
    </QueryClientProvider>
  );
};

export function rootContainer(container: ReactNode) {
  return <RootProvider>{container}</RootProvider>;
}
