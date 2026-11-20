import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { UserFooter } from './user-footer';

const mockTogglePrimarySidebar = jest.fn();
let mockPrimarySidebarCollapsed = false;

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/components/ragflow-avatar', () => ({
  RAGFlowAvatar: () => <div data-testid="user-avatar" />,
}));

jest.mock('@/ee/layouts/components/header-enterprise', () => ({
  EnterpriseHeaderPlanBadge: () => null,
  EnterpriseHeaderTicketsLink: () => null,
  EnterpriseHeaderUpgradeBadge: () => (
    <span data-testid="upgrade-badge">Upgrade</span>
  ),
  EnterpriseSidebarTenantSwitcher: () => null,
}));

jest.mock('@/hooks/use-login-request', () => ({
  useLogout: () => ({ logout: jest.fn() }),
}));

jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchUserInfo: () => ({
    data: {
      avatar: '',
      nickname: 'Test user',
      email: 'user@example.com',
    },
  }),
  useListTenant: () => ({ data: [] }),
}));

jest.mock('./use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    collapsed: mockPrimarySidebarCollapsed,
    toggleCollapsed: mockTogglePrimarySidebar,
  }),
}));

jest.mock('./help-menu', () => ({
  HelpMenu: () => <button aria-label="header.help" data-testid="help-menu" />,
}));

jest.mock('./theme-options', () => ({
  ThemeOptions: () => <div data-testid="theme-options" />,
}));

jest.mock('../bell-button', () => ({
  BellButton: () => null,
}));

function renderUserFooter() {
  return render(
    <MemoryRouter>
      <UserFooter />
    </MemoryRouter>,
  );
}

describe('UserFooter', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPrimarySidebarCollapsed = false;
  });

  it('collapses the primary sidebar from the bottom bar', () => {
    renderUserFooter();

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(mockTogglePrimarySidebar).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('help-menu')).toBeInTheDocument();
    expect(screen.getByTestId('upgrade-badge')).toBeInTheDocument();
  });

  it('keeps account actions and theme separate from the help menu', () => {
    renderUserFooter();

    fireEvent.keyDown(screen.getByTestId('settings-entrypoint'), {
      key: 'ArrowDown',
    });

    expect(
      screen.getByRole('menuitem', { name: 'setting.profile' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('menuitem', { name: 'setting.api' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('menuitem', { name: 'setting.logout' }),
    ).toBeInTheDocument();
    expect(screen.getByTestId('theme-options')).toBeInTheDocument();
    expect(screen.queryByText('sidebar.language')).not.toBeInTheDocument();
    expect(screen.queryByText('header.github')).not.toBeInTheDocument();
    expect(screen.queryByText('header.discord')).not.toBeInTheDocument();
    expect(screen.queryByText('sidebar.version')).not.toBeInTheDocument();
  });

  it('expands from the bottom bar and hides Upgrade when collapsed', () => {
    mockPrimarySidebarCollapsed = true;
    renderUserFooter();

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.expandSidebar' }),
    );

    expect(mockTogglePrimarySidebar).toHaveBeenCalledTimes(1);
    expect(
      screen.queryByRole('button', { name: 'sidebar.collapseSidebar' }),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('settings-entrypoint')).toBeInTheDocument();
    expect(screen.getByTestId('upgrade-badge').parentElement).toHaveClass(
      'hidden',
    );
  });
});
