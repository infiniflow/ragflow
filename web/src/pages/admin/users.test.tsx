import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { listUsers } from '@/services/admin-service';
import AdminUserManagement from './users';

jest.setTimeout(15000);

jest.mock('@/services/admin-service', () => ({
  listUsers: jest.fn(),
  listRoles: jest.fn(),
  createUser: jest.fn(),
  deleteUser: jest.fn(),
  grantSuperuser: jest.fn(),
  revokeSuperuser: jest.fn(),
  updateUserPassword: jest.fn(),
  updateUserRole: jest.fn(),
  updateUserStatus: jest.fn(),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/utils', () => ({ rsaPsw: (value: string) => value }));
jest.mock('@/components/spotlight', () => () => null);

const MockListUsers = jest.mocked(listUsers);
const OriginalResizeObserver = globalThis.ResizeObserver;
const Users = Array.from(
  { length: 25 },
  (_, index): AdminService.ListUsersItem => ({
    email: 'member-' + index.toString().padStart(2, '0') + '@example.com',
    nickname: 'Member ' + index,
    create_date: index,
    is_active: index % 2 ? '0' : '1',
    is_superuser: false,
  }),
);

beforeAll(() => {
  globalThis.ResizeObserver = jest.fn().mockImplementation(() => ({
    observe: jest.fn(),
    unobserve: jest.fn(),
    disconnect: jest.fn(),
  }));
});
afterAll(() => {
  globalThis.ResizeObserver = OriginalResizeObserver;
});
beforeEach(() => {
  jest.clearAllMocks();
  MockListUsers.mockImplementation(async (params) => {
    const filtered = Users.filter(
      (user) =>
        (!params?.keyword || user.email.includes(params.keyword)) &&
        (!params?.status ||
          user.is_active === (params.status === 'active' ? '1' : '0')),
    );
    const offset = ((params?.page ?? 1) - 1) * (params?.page_size ?? 10);
    return {
      data: {
        code: 0,
        message: '',
        total: filtered.length,
        data: filtered.slice(offset, offset + (params?.page_size ?? 10)),
      },
    } as Awaited<ReturnType<typeof MockListUsers>>;
  });
});

function renderUsers(
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  return render(
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <AdminUserManagement />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

it('uses the server total to reach users beyond the first ten and resets the page on search', async () => {
  renderUsers();
  await screen.findByText('member-00@example.com');
  fireEvent.click(screen.getByRole('link', { name: 'Go to next page' }));
  await screen.findByText('member-10@example.com');
  expect(MockListUsers).toHaveBeenLastCalledWith(
    expect.objectContaining({ page: 2, page_size: 10 }),
  );
  expect(screen.queryByText('member-00@example.com')).not.toBeInTheDocument();

  fireEvent.change(screen.getByPlaceholderText('header.search'), {
    target: { value: 'member-24' },
  });
  await waitFor(() =>
    expect(MockListUsers).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, keyword: 'member-24' }),
    ),
  );
  await screen.findByText('member-24@example.com');
  expect(screen.queryByText('member-10@example.com')).not.toBeInTheDocument();
});

it('uses the server order and sends the active-status filter', async () => {
  renderUsers();
  await screen.findByText('member-00@example.com');
  expect(MockListUsers).toHaveBeenLastCalledWith(
    expect.objectContaining({ page: 1, order: 'create_time', sort: 'desc' }),
  );
  fireEvent.click(screen.getByLabelText('admin.status'));
  fireEvent.click(await screen.findByRole('radio', { name: 'admin.inactive' }));
  await waitFor(() =>
    expect(MockListUsers).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, status: 'inactive' }),
    ),
  );
});

it('starts a new search on page one after a page change during debounce', async () => {
  renderUsers();
  await screen.findByText('member-00@example.com');

  const nextPage = () => screen.getByRole('link', { name: 'Go to next page' });
  fireEvent.change(screen.getByPlaceholderText('header.search'), {
    target: { value: 'member-' },
  });
  fireEvent.click(nextPage());

  await waitFor(() =>
    expect(MockListUsers).toHaveBeenCalledWith(
      expect.objectContaining({ page: 2, keyword: '' }),
    ),
  );
  await waitFor(() =>
    expect(
      MockListUsers.mock.calls.find(([params]) => params?.keyword)?.[0],
    ).toEqual(expect.objectContaining({ page: 1, keyword: 'member-' })),
  );
  await screen.findByText('member-00@example.com');
  fireEvent.click(nextPage());
  await screen.findByText('member-10@example.com');
  expect(MockListUsers).toHaveBeenLastCalledWith(
    expect.objectContaining({ page: 2, keyword: 'member-' }),
  );
});

it.each([25, 20])(
  'uses the refreshed total %i to keep the selected page or correct it',
  async (serverTotal) => {
    const client = new QueryClient();
    const listUsers = MockListUsers.getMockImplementation()!;
    let finishRefresh: () => void = () => {};
    const refresh = new Promise<void>((resolve) => {
      finishRefresh = resolve;
    });
    MockListUsers.mockImplementation(async (params) => {
      if (params?.page !== 3) return listUsers(params);
      await refresh;
      const response = await listUsers(params);
      return serverTotal === 25
        ? response
        : {
            ...response,
            data: { ...response.data, data: [], total: serverTotal },
          };
    });
    renderUsers(client);
    await screen.findByText('member-00@example.com');
    const [firstPage] = MockListUsers.mock.calls[0];
    client.setQueryData(['admin/listUsers', { ...firstPage, page: 3 }], {
      code: 0,
      message: '',
      data: [],
      total: 20,
    });

    try {
      fireEvent.click(screen.getByRole('link', { name: 'Go to next page' }));
      await screen.findByText('member-10@example.com');
      fireEvent.click(screen.getByRole('link', { name: 'Go to next page' }));
      await waitFor(() =>
        expect(MockListUsers).toHaveBeenLastCalledWith(
          expect.objectContaining({ page: 3 }),
        ),
      );
    } finally {
      finishRefresh();
    }
    await screen.findByText(
      serverTotal === 25 ? 'member-24@example.com' : 'member-10@example.com',
    );
  },
);
