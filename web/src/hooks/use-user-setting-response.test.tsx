import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import {
  useAgreeTenant,
  useDeleteLangfuseConfig,
  useDeleteTenantUser,
  useFetchSystemVersion,
  useFetchTenantInfo,
  useRemoveSystemToken,
} from './use-user-setting-request';

const mockGetSystemVersion = jest.fn();
const mockGetTenantInfo = jest.fn();
const mockDeleteLangfuseConfig = jest.fn();
const mockRemoveToken = jest.fn();
const mockDeleteTenantUser = jest.fn();
const mockAgreeTenant = jest.fn();
const mockSuccess = jest.fn();
const mockWarning = jest.fn();

jest.mock('@/services/user-service', () => ({
  __esModule: true,
  default: {
    getSystemVersion: () => mockGetSystemVersion(),
    deleteLangfuseConfig: () => mockDeleteLangfuseConfig(),
    removeToken: (...args: unknown[]) => mockRemoveToken(...args),
    getTenantInfo: () => mockGetTenantInfo(),
  },
  deleteTenantUser: (...args: unknown[]) => mockDeleteTenantUser(...args),
  agreeTenant: (...args: unknown[]) => mockAgreeTenant(...args),
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
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: { children?: any }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

beforeEach(() => {
  jest.clearAllMocks();
  mockGetTenantInfo.mockResolvedValue({
    data: { code: 0, data: { tenant_id: 'tenant-1' } },
  });
});

it('exposes tenant query settlement and supports imperative refresh', async () => {
  mockGetTenantInfo
    .mockResolvedValueOnce({
      data: { code: 0, data: { tenant_id: 'tenant-1' } },
    })
    .mockResolvedValueOnce({
      data: { code: 0, data: { tenant_id: 'tenant-2' } },
    });
  const { result } = renderHook(() => useFetchTenantInfo(), {
    wrapper: createWrapper(),
  });

  expect(result.current.isFetched).toBe(false);
  await waitFor(() => expect(result.current.isFetched).toBe(true));
  expect(result.current.data.tenant_id).toBe('tenant-1');

  let refreshedTenantId: string | undefined;
  await act(async () => {
    const refreshed = await result.current.refetch();
    refreshedTenantId = refreshed.data?.tenant_id;
  });
  expect(refreshedTenantId).toBe('tenant-2');
  expect(mockGetTenantInfo).toHaveBeenCalledTimes(2);
  await waitFor(() => expect(result.current.data.tenant_id).toBe('tenant-2'));
});

it('exposes a failed tenant refresh while retaining the last tenant', async () => {
  mockGetTenantInfo
    .mockResolvedValueOnce({
      data: { code: 0, data: { tenant_id: 'tenant-1' } },
    })
    .mockRejectedValueOnce(new Error('tenant refresh failed'));
  const { result } = renderHook(() => useFetchTenantInfo(), {
    wrapper: createWrapper(),
  });

  await waitFor(() => expect(result.current.isFetched).toBe(true));
  expect(result.current.data.tenant_id).toBe('tenant-1');
  expect(result.current.isError).toBe(false);

  await act(async () => {
    const refreshed = await result.current.refetch();
    expect(refreshed.isError).toBe(true);
  });

  await waitFor(() => expect(result.current.isError).toBe(true));
  expect(result.current.isFetched).toBe(true);
  expect(result.current.data.tenant_id).toBe('tenant-1');
});

it('reads the system version from data.version', async () => {
  mockGetSystemVersion.mockResolvedValue({
    data: { code: 0, message: 'success', data: { version: 'v1.2.3' } },
  });
  const { result } = renderHook(() => useFetchSystemVersion());

  await act(async () => {
    await result.current.fetchSystemVersion();
  });

  expect(result.current.version).toBe('v1.2.3');
  expect(result.current.loading).toBe(false);
});

it.each(['success', 'Have not record any Langfuse keys.'])(
  'handles Langfuse deletion without data: %s',
  async (message) => {
    mockDeleteLangfuseConfig.mockResolvedValue({
      data: { code: 0, message },
    });
    const { result } = renderHook(() => useDeleteLangfuseConfig(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      expect(await result.current.deleteLangfuseConfig()).toBe(0);
    });

    expect(mockSuccess).toHaveBeenCalledWith('message.deleted');
    expect(mockWarning).not.toHaveBeenCalled();
  },
);

it.each([0, 102])(
  'uses code %s for acknowledgments without data',
  async (code) => {
    const response = {
      data: { code, message: code === 0 ? 'success' : 'failed' },
    };
    mockRemoveToken.mockResolvedValue(response);
    mockDeleteTenantUser.mockResolvedValue(response);
    mockAgreeTenant.mockResolvedValue(response);
    const { result } = renderHook(
      () => ({
        tokens: useRemoveSystemToken(),
        members: useDeleteTenantUser(),
        invites: useAgreeTenant(),
      }),
      { wrapper: createWrapper() },
    );

    await act(async () => {
      expect(await result.current.tokens.removeToken('token-1')).toBe(code);
      expect(
        await result.current.members.deleteTenantUser({
          userId: 'user-1',
          tenantId: 'tenant-1',
        }),
      ).toBe(code);
      expect(await result.current.invites.agreeTenant('tenant-1')).toBe(code);
    });

    expect(mockRemoveToken).toHaveBeenCalledWith({}, 'token-1');
    expect(mockDeleteTenantUser).toHaveBeenCalledWith({
      tenantId: 'tenant-1',
      userId: 'user-1',
    });
    expect(mockAgreeTenant).toHaveBeenCalledWith('tenant-1');
  },
);
