import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import ApiPage from './index';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/components/api-service/chat-overview-modal/api-content', () => ({
  __esModule: true,
  default: () => <div>API content</div>,
}));

describe('ApiPage', () => {
  it('closes to the location stored in returnTo', () => {
    render(
      <MemoryRouter
        initialEntries={[
          {
            pathname: '/user-setting/api',
            state: { returnTo: '/datasets' },
          },
        ]}
      >
        <Routes>
          <Route path="/user-setting/api" element={<ApiPage />} />
          <Route path="/datasets" element={<div>Datasets page</div>} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText('API content')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'common.close' }));

    expect(screen.getByText('Datasets page')).toBeInTheDocument();
  });
});
