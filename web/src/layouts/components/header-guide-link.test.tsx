import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { USER_GUIDE_URL } from '@/constants/user-guide';
import { Header } from './header';
import { MobileMenuFooter } from './mobile-menu-footer';

const mockOnClose = jest.fn();

jest.mock('@/hooks/logic-hooks', () => ({
  useChangeLanguage: () => jest.fn(),
}));

jest.mock('@/locales/config', () => ({
  supportedLanguages: [{ code: 'ru', displayName: 'Русский' }],
}));

jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchUserInfo: () => ({
    data: { language: 'ru', nickname: 'Тестовый пользователь' },
  }),
  useListTenant: () => ({ data: [] }),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { defaultValue?: string }) =>
      key === 'header.instruction'
        ? 'Инструкция'
        : (options?.defaultValue ?? key),
  }),
}));

jest.mock('./global-navbar', () => ({
  DesktopNavbar: () => <nav aria-label="Основная навигация" />,
  MobileNavbar: () => null,
}));

jest.mock('./theme-button', () => ({
  __esModule: true,
  default: () => <button type="button">Тема</button>,
}));

jest.mock('./bell-button', () => ({
  BellButton: () => <button type="button">Уведомления</button>,
}));

jest.mock('./use-header-nav-layout', () => ({
  useHeaderNavLayout: () => ({
    headerRef: { current: null },
    logoRef: { current: null },
    expandedRightMeasureRef: { current: null },
    navMeasureRef: { current: null },
    isCompact: false,
  }),
}));

describe('user guide entry point', () => {
  beforeEach(() => mockOnClose.mockClear());

  it('replaces the external icon links in the desktop header', () => {
    render(
      <MemoryRouter>
        <Header />
      </MemoryRouter>,
    );

    const guide = screen.getByRole('link', { name: 'Инструкция' });
    expect(guide).toHaveAttribute('href', USER_GUIDE_URL);
    expect(guide).toHaveAttribute('target', '_blank');
    expect(
      document.querySelector('a[href*="discord.com"]'),
    ).not.toBeInTheDocument();
    expect(
      document.querySelector('a[href*="github.com/infiniflow/ragflow"]'),
    ).not.toBeInTheDocument();
  });

  it('shows the same guide link in the mobile menu footer', () => {
    render(<MobileMenuFooter onClose={mockOnClose} />);

    const guide = screen.getByRole('link', { name: 'Инструкция' });
    expect(guide).toHaveAttribute('href', USER_GUIDE_URL);
    expect(screen.queryByText('Discord')).not.toBeInTheDocument();
    expect(screen.queryByText('GitHub')).not.toBeInTheDocument();

    guide.click();
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });
});
