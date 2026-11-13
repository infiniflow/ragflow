import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import AdminModelSettings from '../model-settings';

const mockGet = jest.fn();
const mockSet = jest.fn();
const mockNotify = { success: jest.fn(), error: jest.fn() };

jest.mock('@/services/admin-service', () => ({
  getLlmMaxRetries: (...args: unknown[]) => mockGet(...args),
  setLlmMaxRetries: (...args: unknown[]) => mockSet(...args),
}));
jest.mock('@/utils/notification', () => ({
  __esModule: true,
  default: {
    success: (...args: unknown[]) => mockNotify.success(...args),
    error: (...args: unknown[]) => mockNotify.error(...args),
  },
}));
jest.mock('@/components/spotlight', () => () => null);

const stored = (value: string) => ({
  data: { code: 0, message: '', data: [{ name: 'llm.max_retries', value }] },
});

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <AdminModelSettings />
    </QueryClientProvider>,
  );
}

const field = () =>
  screen.findByLabelText('admin.modelSettingsPage.maxRetries');
const saveButton = () => screen.getByRole('button', { name: 'common.save' });

beforeEach(() => {
  mockGet.mockReset();
  mockSet.mockReset();
  mockNotify.success.mockReset();
  mockNotify.error.mockReset();
});

it('shows the saved override and saves a changed value as a number', async () => {
  mockGet.mockResolvedValue(stored('4'));
  mockSet.mockResolvedValue({ data: { code: 0, message: '' } });
  renderPage();

  const input = await field();
  expect(input).toHaveValue(4);
  expect(saveButton()).toBeDisabled();

  fireEvent.change(input, { target: { value: '6' } });
  expect(saveButton()).toBeEnabled();
  fireEvent.click(saveButton());

  await waitFor(() => expect(mockSet).toHaveBeenCalledWith(6));
  await waitFor(() => expect(mockNotify.success).toHaveBeenCalled());
});

it('treats an empty field as no override and saves it as null', async () => {
  mockGet.mockResolvedValue(stored('4'));
  mockSet.mockResolvedValue({ data: { code: 0, message: '' } });
  renderPage();

  fireEvent.change(await field(), { target: { value: '' } });
  fireEvent.click(saveButton());

  await waitFor(() => expect(mockSet).toHaveBeenCalledWith(null));
});

it.each([
  ['11', 'admin.modelSettingsPage.errorOutOfRange'],
  ['-1', 'admin.modelSettingsPage.errorOutOfRange'],
  ['2.5', 'admin.modelSettingsPage.errorNotInteger'],
])('rejects %s without calling the API', async (value, errorKey) => {
  mockGet.mockResolvedValue(stored(''));
  renderPage();

  fireEvent.change(await field(), { target: { value } });

  expect(screen.getByText(errorKey)).toBeInTheDocument();
  expect(saveButton()).toBeDisabled();
  fireEvent.click(saveButton());
  expect(mockSet).not.toHaveBeenCalled();
});

it('reports a failed save and keeps the edit', async () => {
  mockGet.mockResolvedValue(stored('4'));
  mockSet.mockResolvedValue({ data: { code: 500, message: 'database down' } });
  renderPage();

  const input = await field();
  fireEvent.change(input, { target: { value: '8' } });
  fireEvent.click(saveButton());

  await waitFor(() =>
    expect(mockNotify.error).toHaveBeenCalledWith({
      message: 'admin.modelSettingsPage.saveFailed',
      description: 'database down',
    }),
  );
  expect(input).toHaveValue(8);
});

it('offers Retry when the setting cannot be loaded', async () => {
  mockGet.mockRejectedValueOnce(new Error('network down'));
  mockGet.mockResolvedValueOnce(stored('2'));
  renderPage();

  expect(await screen.findByRole('alert')).toHaveTextContent('network down');
  fireEvent.click(screen.getByRole('button', { name: 'common.retry' }));

  expect(await field()).toHaveValue(2);
  expect(mockGet).toHaveBeenCalledTimes(2);
});
