import { ThemeEnum } from '@/constants/common';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { ThemeProvider, useIsDarkTheme, useTheme } from './theme-provider';

let mediaQueryMatches = false;
let mediaQueryListener: (() => void) | undefined;

const mockMediaQuery = {
  get matches() {
    return mediaQueryMatches;
  },
  addEventListener: jest.fn((_event: string, listener: () => void) => {
    mediaQueryListener = listener;
  }),
  removeEventListener: jest.fn(),
};

function renderTheme(defaultTheme: ThemeEnum) {
  return renderHook(() => ({ ...useTheme(), isDark: useIsDarkTheme() }), {
    wrapper: ({ children }: PropsWithChildren) => (
      <ThemeProvider defaultTheme={defaultTheme} storageKey="test-theme">
        {children}
      </ThemeProvider>
    ),
  });
}

describe('ThemeProvider system theme', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    document.documentElement.classList.remove(ThemeEnum.Light, ThemeEnum.Dark);
    mediaQueryMatches = false;
    mediaQueryListener = undefined;
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      writable: true,
      value: jest.fn(() => mockMediaQuery),
    });
  });

  it('resolves the system theme and follows OS changes', async () => {
    mediaQueryMatches = true;
    const { result } = renderTheme(ThemeEnum.System);

    await waitFor(() => expect(result.current.resolvedTheme).toBe('dark'));
    expect(result.current.isDark).toBe(true);
    expect(document.documentElement).toHaveClass('dark');
    expect(localStorage.getItem('test-theme')).toBe('system');

    act(() => {
      mediaQueryMatches = false;
      mediaQueryListener?.();
    });

    expect(result.current.theme).toBe(ThemeEnum.System);
    expect(result.current.resolvedTheme).toBe(ThemeEnum.Light);
    expect(result.current.isDark).toBe(false);
    expect(document.documentElement).toHaveClass('light');
    expect(document.documentElement).not.toHaveClass('dark');
  });

  it('keeps an explicit light theme when the OS theme changes', async () => {
    mediaQueryMatches = true;
    const { result } = renderTheme(ThemeEnum.Light);

    await waitFor(() => expect(document.documentElement).toHaveClass('light'));

    act(() => {
      mediaQueryMatches = false;
      mediaQueryListener?.();
    });

    expect(result.current.theme).toBe(ThemeEnum.Light);
    expect(result.current.resolvedTheme).toBe(ThemeEnum.Light);
    expect(result.current.isDark).toBe(false);
    expect(document.documentElement).not.toHaveClass('dark');
  });
});
