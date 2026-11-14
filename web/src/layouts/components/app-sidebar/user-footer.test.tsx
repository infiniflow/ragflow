import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { UserFooter } from './user-footer';

const mockTogglePrimarySidebar = jest.fn();
let mockPrimarySidebarCollapsed = false;

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/components/theme-provider', () => ({
  useIsDarkTheme: () => false,
  useTheme: () => ({ theme: 'light', setTheme: jest.fn() }),
}));

jest.mock('@/components/ragflow-avatar', () => ({
  RAGFlowAvatar: () => <div data-testid="user-avatar" />,
}));

jest.mock('@/ee/layouts/components/header-enterprise', () => ({
  EnterpriseHeaderPlanBadge: () => null,
  EnterpriseHeaderTicketsLink: () => null,
}));

jest.mock('@/hooks/logic-hooks', () => ({
  useChangeLanguage: () => jest.fn(),
}));

jest.mock('@/hooks/use-login-request', () => ({
  useLogout: () => ({ logout: jest.fn() }),
}));

jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchSystemVersion: () => ({
    version: '',
    fetchSystemVersion: jest.fn(),
  }),
  useFetchUserInfo: () => ({
    data: {
      language: 'en',
      avatar: '',
      nickname: 'Test user',
      email: 'user@example.com',
    },
  }),
  useListTenant: () => ({ data: [] }),
}));

jest.mock('@/locales/config', () => ({
  supportedLanguages: [],
}));

jest.mock('./use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    collapsed: mockPrimarySidebarCollapsed,
    toggleCollapsed: mockTogglePrimarySidebar,
  }),
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

describe('UserFooter primary sidebar control', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPrimarySidebarCollapsed = false;
  });

  it('collapses the primary sidebar from the button beside the avatar', () => {
    renderUserFooter();

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(mockTogglePrimarySidebar).toHaveBeenCalledTimes(1);
  });

  it('does not render the collapse button when the primary sidebar is collapsed', () => {
    mockPrimarySidebarCollapsed = true;
    renderUserFooter();

    expect(
      screen.queryByRole('button', { name: 'sidebar.collapseSidebar' }),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('settings-entrypoint')).toBeInTheDocument();
  });
});
