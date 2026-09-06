import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import Home from '.';

const mockUseSystemConfig = jest.fn();

jest.mock('@/hooks/use-system-request', () => ({
  useSystemConfig: () => mockUseSystemConfig(),
}));

jest.mock('./applications', () => ({
  Applications: () => <div>Home applications</div>,
}));

jest.mock('./banner', () => ({
  NextBanner: () => <div>Home banner</div>,
}));

jest.mock('./datasets', () => ({
  Datasets: () => <div>Home datasets</div>,
}));

const renderHome = () =>
  render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/datasets" element={<div>Datasets page</div>} />
        <Route path="/chats" element={<div>Chats page</div>} />
      </Routes>
    </MemoryRouter>,
  );

describe('Home landing route', () => {
  it('renders home when the section is available', () => {
    mockUseSystemConfig.mockReturnValue({
      config: { registerEnabled: 1, visibleSections: ['home', 'dataset'] },
      loading: false,
    });

    renderHome();

    expect(screen.getByText('Home banner')).toBeInTheDocument();
  });

  it('redirects to the first available navigation section', () => {
    mockUseSystemConfig.mockReturnValue({
      config: { registerEnabled: 1, visibleSections: ['chat', 'dataset'] },
      loading: false,
    });

    renderHome();

    expect(screen.getByText('Datasets page')).toBeInTheDocument();
    expect(screen.queryByText('Home banner')).not.toBeInTheDocument();
  });
});
