import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ThemeEnum } from '@/constants/common';
import { fireEvent, render, screen } from '@testing-library/react';
import { ThemeOptions } from './theme-options';

const mockSetTheme = jest.fn();
let mockTheme = ThemeEnum.Dark;

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/components/theme-provider', () => ({
  useTheme: () => ({ theme: mockTheme, setTheme: mockSetTheme }),
}));

function renderThemeOptions() {
  return render(
    <DropdownMenu open>
      <DropdownMenuTrigger>Open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <ThemeOptions />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
}

describe('ThemeOptions', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockTheme = ThemeEnum.Dark;
  });

  it.each([
    ['sidebar.darkTheme', ThemeEnum.Dark],
    ['sidebar.lightTheme', ThemeEnum.Light],
    ['sidebar.systemTheme', ThemeEnum.System],
  ])('selects %s', (label, theme) => {
    renderThemeOptions();

    fireEvent.click(screen.getByRole('menuitemradio', { name: label }));

    expect(mockSetTheme).toHaveBeenCalledWith(theme);
  });

  it('marks the selected theme as checked', () => {
    mockTheme = ThemeEnum.System;
    renderThemeOptions();

    expect(
      screen.getByRole('menuitemradio', { name: 'sidebar.systemTheme' }),
    ).toHaveAttribute('aria-checked', 'true');
    expect(
      screen.getByRole('menuitemradio', { name: 'sidebar.darkTheme' }),
    ).toHaveAttribute('aria-checked', 'false');
  });
});
