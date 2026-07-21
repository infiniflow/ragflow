import { ThemeEnum } from '@/constants/common';
import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';

type ThemeProviderProps = {
  children: React.ReactNode;
  defaultTheme?: ThemeEnum;
  storageKey?: string;
};

type ThemeProviderState = {
  theme: ThemeEnum;
  setTheme: (theme: ThemeEnum, persist?: boolean) => void;
};

const initialState: ThemeProviderState = {
  theme: ThemeEnum.Light,
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
  const persistRef = useRef(true);

  const setTheme = useCallback((nextTheme: ThemeEnum, persist = true) => {
    persistRef.current = persist;
    setThemeState(nextTheme);
  }, []);

  useEffect(() => {
    const root = window.document.documentElement;
    root.classList.remove(ThemeEnum.Light, ThemeEnum.Dark);
    if (persistRef.current) {
      localStorage.setItem(storageKey, theme);
    }
    root.classList.add(theme);
  }, [storageKey, theme]);

  return (
    <ThemeProviderContext.Provider
      {...props}
      value={{
        theme,
        setTheme,
      }}
    >
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
  const { theme } = useTheme();

  return theme === ThemeEnum.Dark;
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
