import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import {
  useDeleteLangfuseConfig,
  useFetchSystemVersion,
} from './use-user-setting-request';

const mockGetSystemVersion = jest.fn();
const mockDeleteLangfuseConfig = jest.fn();
const mockSuccess = jest.fn();
const mockWarning = jest.fn();

jest.mock('@/services/user-service', () => ({
  __esModule: true,
  default: {
    getSystemVersion: () => mockGetSystemVersion(),
    deleteLangfuseConfig: () => mockDeleteLangfuseConfig(),
  },
}));
jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: {},
}));
jest.mock('./use-llm-request', () => ({
  useFetchDefaultModelDictionary: jest.fn(),
}));
jest.mock('@/locales/config', () => ({
  DEFAULT_LANGUAGE_CODE: 'en',
  supportedLanguages: [],
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: {
    success: (value: string) => mockSuccess(value),
    warning: (value: string) => mockWarning(value),
  },
}));

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children?: any }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

beforeEach(() => jest.clearAllMocks());

it('reads the system version from data', async () => {
  mockGetSystemVersion.mockResolvedValue({
    data: { code: 0, message: 'success', data: 'v1.2.3' },
  });
  const { result } = renderHook(() => useFetchSystemVersion());

  await act(async () => {
    await result.current.fetchSystemVersion();
  });

  expect(result.current.version).toBe('v1.2.3');
  expect(result.current.loading).toBe(false);
});

it.each([
  ['success', true, 'deleted'],
  ['Have not record any Langfuse keys.', null, 'noLangfuseConfigToDelete'],
])('handles Langfuse deletion message %s', async (message, data, key) => {
  mockDeleteLangfuseConfig.mockResolvedValue({
    data: { code: 0, message, ...(data === null ? {} : { data }) },
  });
  const { result } = renderHook(() => useDeleteLangfuseConfig(), {
    wrapper: createWrapper(),
  });

  await act(async () => {
    expect(await result.current.deleteLangfuseConfig()).toBe(0);
  });

  const notification = message === 'success' ? mockSuccess : mockWarning;
  expect(notification).toHaveBeenCalledWith(`message.${key}`);
});
