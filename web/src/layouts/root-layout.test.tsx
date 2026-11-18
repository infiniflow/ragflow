import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import RootLayout from './root-layout';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/ee/layouts/components/header-enterprise', () => ({
  EnterpriseHeaderUpgradeBadge: () => null,
}));

jest.mock('./components/app-sidebar/nav-item', () => ({
  IntegrationsNavGroup: () => null,
  NavItemLink: () => null,
}));

jest.mock('./components/app-sidebar/user-footer', () => ({
  UserFooter: () => null,
}));

describe('RootLayout sidebar providers', () => {
  it('renders AppSidebar with the agent sidebar collapse provider', () => {
    render(
      <MemoryRouter initialEntries={['/agent/agent-1']}>
        <RootLayout />
      </MemoryRouter>,
    );

    expect(screen.getByRole('link', { name: 'RAGFlow' })).toBeInTheDocument();
  });
});
