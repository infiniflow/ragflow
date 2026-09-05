import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { DesktopNavbar } from './global-navbar';

const mockUseSystemConfig = jest.fn();

jest.mock('@/hooks/use-system-request', () => ({
  useSystemConfig: () => mockUseSystemConfig(),
}));

jest.mock('@/utils/css-support', () => ({
  supportsCssAnchor: false,
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { defaultValue?: string }) =>
      options?.defaultValue ?? key,
  }),
}));

describe('DesktopNavbar', () => {
  it('shows only globally visible sections, including home', () => {
    mockUseSystemConfig.mockReturnValue({
      config: {
        registerEnabled: 1,
        visibleSections: ['chat', 'agent'],
      },
      loading: false,
    });

    render(
      <MemoryRouter>
        <DesktopNavbar />
      </MemoryRouter>,
    );

    expect(screen.queryByTestId('nav-home')).not.toBeInTheDocument();
    expect(screen.getByTestId('nav-chat')).toBeInTheDocument();
    expect(screen.getByTestId('nav-agent')).toBeInTheDocument();
    expect(screen.queryByTestId('nav-search')).not.toBeInTheDocument();
    expect(screen.queryByTestId('nav-openmetadata')).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('nav-business-documents'),
    ).not.toBeInTheDocument();
  });
});
