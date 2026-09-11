/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { Toaster as Sonner } from '@/components/ui/sonner';
import { Toaster } from '@/components/ui/toaster';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { configResponsive } from 'ahooks';
import dayjs from 'dayjs';
import 'dayjs/locale/ar';
import 'dayjs/locale/tr';
import 'dayjs/locale/zh-cn';
import advancedFormat from 'dayjs/plugin/advancedFormat';
import customParseFormat from 'dayjs/plugin/customParseFormat';
import localeData from 'dayjs/plugin/localeData';
import weekOfYear from 'dayjs/plugin/weekOfYear';
import weekYear from 'dayjs/plugin/weekYear';
import weekday from 'dayjs/plugin/weekday';
import React from 'react';
import { RouterProvider } from 'react-router';
import { ThemeProvider } from './components/theme-provider';
import { TooltipProvider } from './components/ui/tooltip';
import { ThemeEnum } from './constants/common';
import { routers } from './routes';

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
        logOnDifferentValues: false,
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
  return (
    <>
      {children}

      <Sonner position="top-right" expand richColors closeButton />

      <Toaster />
    </>
  );
}

const RootProvider = ({ children }: React.PropsWithChildren) => {
  return (
    <TooltipProvider>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider
          defaultTheme={ThemeEnum.Dark}
          storageKey="ragflow-ui-theme"
        >
          <Root>{children}</Root>
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
