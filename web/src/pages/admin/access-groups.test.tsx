import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import {
  createAccessGroup,
  deleteAccessGroup,
  getAccessGroupOptions,
  listAccessGroups,
  updateAccessGroup,
} from '@/services/admin-service';
import AdminAccessGroups from './access-groups';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key.split('.').pop() }),
}));
jest.mock('@/services/admin-service', () => ({
  createAccessGroup: jest.fn(),
  deleteAccessGroup: jest.fn(),
  getAccessGroupOptions: jest.fn(),
  listAccessGroups: jest.fn(),
  updateAccessGroup: jest.fn(),
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: jest.fn() },
}));

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AdminAccessGroups />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  jest
    .mocked(listAccessGroups)
    .mockResolvedValue({ data: { data: { items: [] } } } as never);
  jest.mocked(getAccessGroupOptions).mockResolvedValue({
    data: {
      data: {
        users: [
          {
            id: 'u1',
            email: 'user@example.com',
            nickname: 'User One',
            is_superuser: false,
          },
        ],
        datasets: [{ id: 'kb1', name: 'Policies', tenant_id: 'admin' }],
        sections: ['dataset', 'business_documents'],
      },
    },
  } as never);
  jest
    .mocked(createAccessGroup)
    .mockResolvedValue({ data: { data: {} } } as never);
});

test('creates a group with selected users, sources and sections', async () => {
  mount();
  fireEvent.change(await screen.findByLabelText('name'), {
    target: { value: 'Legal' },
  });
  fireEvent.click(await screen.findByText('User One'));
  fireEvent.click(screen.getByText('Policies'));
  fireEvent.click(screen.getByText('dataset'));
  fireEvent.click(screen.getByRole('button', { name: 'save' }));

  await waitFor(() =>
    expect(createAccessGroup).toHaveBeenCalledWith({
      name: 'Legal',
      description: '',
      user_ids: ['u1'],
      dataset_ids: ['kb1'],
      sections: ['dataset'],
    }),
  );
  expect(deleteAccessGroup).not.toHaveBeenCalled();
  expect(updateAccessGroup).not.toHaveBeenCalled();
});
