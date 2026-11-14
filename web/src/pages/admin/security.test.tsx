import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { changeOwnPassword } from '@/services/admin-service';
import { adminStorage } from '@/utils/authorization-util';
import { rsaPsw } from '@/utils';
import AdminSecurity from './security';

const MockNavigate = jest.fn();
const MockChangeOwnPassword = jest.mocked(changeOwnPassword);
jest.mock('react-router', () => ({ useNavigate: () => MockNavigate }));
jest.mock('@/services/admin-service', () => ({ changeOwnPassword: jest.fn() }));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/utils', () => ({
  rsaPsw: jest.fn((value: string) => 'encrypted:' + value),
}));

beforeEach(() => jest.clearAllMocks());

it('requires the current password and clears only admin state after a successful change', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(['admin', 'audit'], ['fixture']);
  client.setQueryData(['datasets'], ['fixture']);
  const clearAdmin = jest.spyOn(adminStorage, 'removeAll');
  jest
    .mocked(changeOwnPassword)
    .mockResolvedValue({ data: { code: 0 } } as Awaited<
      ReturnType<typeof MockChangeOwnPassword>
    >);
  render(
    <QueryClientProvider client={client}>
      <AdminSecurity />
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByLabelText('admin.newPassword'), {
    target: { value: 'replacement-fixture' },
  });
  fireEvent.change(screen.getByLabelText('admin.confirmNewPassword'), {
    target: { value: 'replacement-fixture' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'admin.changePassword' }));
  await screen.findByText('adminEnhancements.currentPasswordRequired');
  expect(changeOwnPassword).not.toHaveBeenCalled();

  fireEvent.change(screen.getByLabelText('adminEnhancements.currentPassword'), {
    target: { value: 'current-fixture' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'admin.changePassword' }));
  await waitFor(() =>
    expect(MockNavigate).toHaveBeenCalledWith('/admin', { replace: true }),
  );
  expect(changeOwnPassword).toHaveBeenCalledWith(
    'encrypted:current-fixture',
    'encrypted:replacement-fixture',
  );
  expect(clearAdmin).toHaveBeenCalledTimes(1);
  expect(client.getQueryData(['admin', 'audit'])).toBeUndefined();
  expect(client.getQueryData(['datasets'])).toEqual(['fixture']);
  clearAdmin.mockRestore();
});

it('shows encryption failures without sending an invalid password payload', async () => {
  jest.mocked(rsaPsw).mockReturnValue(false);
  const client = new QueryClient();
  render(
    <QueryClientProvider client={client}>
      <AdminSecurity />
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByLabelText('adminEnhancements.currentPassword'), {
    target: { value: 'current-fixture' },
  });
  fireEvent.change(screen.getByLabelText('admin.newPassword'), {
    target: { value: 'replacement-fixture' },
  });
  fireEvent.change(screen.getByLabelText('admin.confirmNewPassword'), {
    target: { value: 'replacement-fixture' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'admin.changePassword' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'adminEnhancements.passwordEncryptionFailed',
  );
  expect(changeOwnPassword).not.toHaveBeenCalled();
});
