import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useFetchFutureAGIConfig } from '@/hooks/use-user-setting-request';
import {
  FutureAGIConfigurationForm,
  FormId,
} from './futureagi-configuration-form';

jest.mock('@/hooks/use-user-setting-request', () => ({
  useFetchFutureAGIConfig: jest.fn(),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

// jsdom does not implement the observer used by the real Radix switch.
const OriginalResizeObserver = globalThis.ResizeObserver;
beforeAll(() => {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});
afterAll(() => {
  globalThis.ResizeObserver = OriginalResizeObserver;
});

const MockFetch = jest.mocked(useFetchFutureAGIConfig);
const SavedConfig = {
  tenant_id: 'tenant-1',
  api_key: '****1234',
  secret_key: '****5678',
  project_name: 'chat-traces',
  endpoint: 'https://api.futureagi.com',
  redact: false,
};

function renderForm() {
  const save = jest.fn();
  render(
    <>
      <FutureAGIConfigurationForm onOk={save} />
      <button type="submit" form={FormId}>
        Save
      </button>
    </>,
  );
  return save;
}

it('keeps saved secrets blank and saves privacy changes without resubmitting masks', async () => {
  MockFetch.mockReturnValue({
    data: SavedConfig,
    loading: false,
    isError: false,
  });
  const save = renderForm();
  expect(screen.getByLabelText('setting.futureAGIApiKey')).toHaveValue('');
  expect(screen.getByLabelText('setting.secretKey')).toHaveValue('');
  fireEvent.click(screen.getByRole('switch'));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
  expect(save.mock.calls[0][0]).toEqual({
    api_key: '',
    secret_key: '',
    project_name: SavedConfig.project_name,
    endpoint: SavedConfig.endpoint,
    redact: true,
  });
});

it('requires credentials on first save and captures content by default', async () => {
  MockFetch.mockReturnValue({ data: null, loading: false, isError: false });
  const save = renderForm();
  expect(screen.getByRole('switch')).not.toBeChecked();
  fireEvent.change(screen.getByLabelText('setting.futureAGIProjectName'), {
    target: { value: 'chat-traces' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(
      screen.getByText('setting.futureAGIApiKeyMessage'),
    ).toBeInTheDocument(),
  );
  expect(save).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText('setting.futureAGIApiKey'), {
    target: { value: 'test-key' },
  });
  fireEvent.change(screen.getByLabelText('setting.secretKey'), {
    target: { value: 'test-secret' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
  expect(save.mock.calls[0][0].redact).toBe(false);
});

it.each([
  { loading: true, isError: false, role: 'status' },
  { loading: false, isError: true, role: 'alert' },
])(
  'does not show an editable form while loading or after a fetch failure',
  ({ loading, isError, role }) => {
    MockFetch.mockReturnValue({ data: undefined, loading, isError });
    renderForm();
    expect(screen.getByRole(role)).toBeInTheDocument();
    expect(
      screen.queryByLabelText('setting.futureAGIApiKey'),
    ).not.toBeInTheDocument();
  },
);
