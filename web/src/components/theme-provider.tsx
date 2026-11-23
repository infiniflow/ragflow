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

import { ThemeEnum } from '@/constants/common';
import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

type ThemeProviderProps = {
  children: React.ReactNode;
  defaultTheme?: ThemeEnum;
  storageKey?: string;
};

type ResolvedTheme = Exclude<ThemeEnum, ThemeEnum.System>;

type ThemeProviderState = {
  theme: ThemeEnum;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: ThemeEnum, persist?: boolean) => void;
};

const SystemThemeQuery = '(prefers-color-scheme: dark)';

const getSystemTheme = (): ResolvedTheme =>
  typeof window.matchMedia === 'function' &&
  window.matchMedia(SystemThemeQuery).matches
    ? ThemeEnum.Dark
    : ThemeEnum.Light;

const initialState: ThemeProviderState = {
  theme: ThemeEnum.Light,
  resolvedTheme: ThemeEnum.Light,
  setTheme: () => null,
};

const ThemeProviderContext = createContext<ThemeProviderState>(initialState);

export function ThemeProvider({
  children,
  defaultTheme = ThemeEnum.Dark,
  storageKey = 'vite-ui-theme',
  ...props
}: ThemeProviderProps) {
  const [theme, setThemeState] = useState<ThemeEnum>(
    () => (localStorage.getItem(storageKey) as ThemeEnum) || defaultTheme,
  );
  const [systemTheme, setSystemTheme] = useState<ResolvedTheme>(getSystemTheme);
  const persistRef = useRef(true);
  const resolvedTheme = theme === ThemeEnum.System ? systemTheme : theme;

  const setTheme = useCallback((nextTheme: ThemeEnum, persist = true) => {
    persistRef.current = persist;
    setThemeState(nextTheme);
  }, []);

  const contextValue = useMemo(
    () => ({ theme, resolvedTheme, setTheme }),
    [theme, resolvedTheme, setTheme],
  );

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return;

    const mediaQuery = window.matchMedia(SystemThemeQuery);
    const updateSystemTheme = () => {
      setSystemTheme(mediaQuery.matches ? ThemeEnum.Dark : ThemeEnum.Light);
    };

    updateSystemTheme();
    mediaQuery.addEventListener('change', updateSystemTheme);
    return () => mediaQuery.removeEventListener('change', updateSystemTheme);
  }, []);

  useEffect(() => {
    const root = window.document.documentElement;
    root.classList.remove(ThemeEnum.Light, ThemeEnum.Dark);
    root.classList.add(resolvedTheme);
  }, [resolvedTheme]);

  useEffect(() => {
    if (persistRef.current) {
      localStorage.setItem(storageKey, theme);
    }
  }, [storageKey, theme]);

  return (
    <ThemeProviderContext.Provider {...props} value={contextValue}>
      {children}
    </ThemeProviderContext.Provider>
  );
}

export const useTheme = () => {
  const context = useContext(ThemeProviderContext);

  if (context === undefined)
    throw new Error('useTheme must be used within a ThemeProvider');

  return context;
};

export const useIsDarkTheme = () => {
  const { resolvedTheme } = useTheme();

  return resolvedTheme === ThemeEnum.Dark;
};

export function useSwitchToDarkThemeOnMount() {
  const { setTheme } = useTheme();

  useEffect(() => {
    setTheme(ThemeEnum.Dark);
  }, [setTheme]);
}

export function useSyncThemeFromParams(theme: string | null) {
  const { theme: contextTheme, setTheme } = useTheme();
  const originalThemeRef = useRef<ThemeEnum | null>(null);

  useEffect(() => {
    if (theme && (theme === ThemeEnum.Light || theme === ThemeEnum.Dark)) {
      if (originalThemeRef.current === null) {
        originalThemeRef.current = contextTheme;
      }
      setTheme(theme as ThemeEnum, false);
    }
  }, [theme, contextTheme, setTheme]);

  useEffect(() => {
    return () => {
      if (originalThemeRef.current !== null) {
        setTheme(originalThemeRef.current, false);
      }
    };
  }, [setTheme]);
}
