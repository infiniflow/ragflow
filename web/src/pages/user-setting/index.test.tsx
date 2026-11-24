import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import UserSetting from './index';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('./sidebar', () => ({
  SideBar: () => <div>Settings sidebar</div>,
}));

describe('UserSetting', () => {
  it('closes to the location stored in returnTo', () => {
    render(
      <MemoryRouter
        initialEntries={[
          {
            pathname: '/user-setting/profile',
            state: { returnTo: '/datasets' },
          },
        ]}
      >
        <Routes>
          <Route path="/user-setting/profile" element={<UserSetting />} />
          <Route path="/datasets" element={<div>Datasets page</div>} />
        </Routes>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'common.close' }));

    expect(screen.getByText('Datasets page')).toBeInTheDocument();
  });
});
