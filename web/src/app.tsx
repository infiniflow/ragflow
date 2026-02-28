import { Toaster as Sonner } from '@/components/ui/sonner';
import { Toaster } from '@/components/ui/toaster';
import i18n, { changeLanguageAsync } from '@/locales/config';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { configResponsive } from 'ahooks';
import dayjs from 'dayjs';
import advancedFormat from 'dayjs/plugin/advancedFormat';
import customParseFormat from 'dayjs/plugin/customParseFormat';
import localeData from 'dayjs/plugin/localeData';
import weekOfYear from 'dayjs/plugin/weekOfYear';
import weekYear from 'dayjs/plugin/weekYear';
import weekday from 'dayjs/plugin/weekday';
import React, { useEffect } from 'react';
import { RouterProvider } from 'react-router';
import { ThemeProvider } from './components/theme-provider';
import { SidebarProvider } from './components/ui/sidebar';
import { TooltipProvider } from './components/ui/tooltip';
import { ThemeEnum } from './constants/common';
import { routers } from './routes';
import storage from './utils/authorization-util';

import 'react-photo-view/dist/react-photo-view.css';

configResponsive({
  sm: 640,
  md: 768,
  lg: 1024,
  xl: 1280,
  '2xl': 1536,
  '3xl': 1780,
  '4xl': 1980,
});

dayjs.extend(customParseFormat);
dayjs.extend(advancedFormat);
dayjs.extend(weekday);
dayjs.extend(localeData);
dayjs.extend(weekOfYear);
dayjs.extend(weekYear);

if (process.env.NODE_ENV === 'development') {
  import('@welldone-software/why-did-you-render').then(
    (whyDidYouRenderModule) => {
      const whyDidYouRender = whyDidYouRenderModule.default;
      whyDidYouRender(React, {
        trackAllPureComponents: true,
        trackExtraHooks: [],
        logOnDifferentValues: true,
        exclude: [/^RouterProvider$/],
      });
    },
  );
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 2,
    },
  },
});

function Root({ children }: React.PropsWithChildren) {
  useEffect(() => {
    const lng = storage.getLanguage();
    if (lng) {
      document.documentElement.lang = lng;
    }
  }, []);

  useEffect(() => {
    const handleLanguageChanged = (lng: string) => {
      storage.setLanguage(lng);
      document.documentElement.lang = lng;
    };

    i18n.on('languageChanged', handleLanguageChanged);

    return () => {
      i18n.off('languageChanged', handleLanguageChanged);
    };
  }, []);

  return (
    <SidebarProvider className="h-full">
      <div className="w-full h-dvh relative">{children}</div>
    </SidebarProvider>
  );
}

const RootProvider = ({ children }: React.PropsWithChildren) => {
  useEffect(() => {
    const lng = storage.getLanguage();
    if (lng) {
      changeLanguageAsync(lng);
    }
  }, []);

  return (
    <TooltipProvider>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider
          defaultTheme={ThemeEnum.Dark}
          storageKey="ragflow-ui-theme"
        >
          <Root>{children}</Root>
          <Sonner position={'top-right'} expand richColors closeButton></Sonner>
          <Toaster />
        </ThemeProvider>
      </QueryClientProvider>
    </TooltipProvider>
  );
};

const RouterProviderWrapper: React.FC<{ router: typeof routers }> = ({
  router,
}) => {
  return <RouterProvider router={router}></RouterProvider>;
};
RouterProviderWrapper.whyDidYouRender = false;

export default function AppContainer() {
  return (
    <RootProvider>
      <RouterProviderWrapper router={routers} />
    </RootProvider>
  );
}
