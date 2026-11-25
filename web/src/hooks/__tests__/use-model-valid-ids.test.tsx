import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useModelValidIds } from '../use-llm-request';

const mockListAllAddedModels = jest.fn();
const mockUseAdditionalModelValidIds = jest.fn();
const mockResolveAdditionalValidIds = jest.fn();
const mockGetAdditionalPendingMessage = jest.fn();

jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: {
    listAllAddedModels: (...args: unknown[]) => mockListAllAddedModels(...args),
  },
}));

jest.mock('@/ee/hooks/use-additional-model-valid-ids', () => ({
  useAdditionalModelValidIds: (options: unknown) =>
    mockUseAdditionalModelValidIds(options),
}));

jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: (key: string) => key }),
}));

const groupId = '0123456789abcdef0123456789abcdef';
const groupRef = `gp-${groupId}`;
const directModel = {
  model_id: 'model-1',
  status: 'active',
  instance_status: 'active',
  model_type: ['chat'],
  name: 'gpt-4',
  provider_id: 'provider-1',
  provider_name: 'OpenAI',
  instance_id: 'instance-1',
  instance_name: 'default',
};
const freshDirectModel = {
  ...directModel,
  model_id: 'model-2',
  name: 'gpt-4o',
};

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return {
    queryClient,
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    ),
  };
}

function mockAdditional(validIds = new Set([groupRef])) {
  mockUseAdditionalModelValidIds.mockReturnValue({
    validIds,
    isSettled: true,
    getPendingMessage: mockGetAdditionalPendingMessage,
    resolveValidIds: mockResolveAdditionalValidIds,
  });
}

describe('useModelValidIds', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockListAllAddedModels.mockResolvedValue({
      data: { code: 0, data: [directModel] },
    });
    mockResolveAdditionalValidIds.mockResolvedValue(new Set([groupRef]));
    mockAdditional();
  });

  it('unions direct and group ids and freshly resolves both sources', async () => {
    const { wrapper, queryClient } = createWrapper();
    const { result } = renderHook(() => useModelValidIds(['chat']), {
      wrapper,
    });

    await waitFor(() => {
      expect(result.current.isFetched).toBe(true);
      expect(result.current.isSettled).toBe(true);
    });
    expect(result.current.validIds).toEqual(
      new Set(['model-1', 'gpt-4@default@OpenAI', groupRef]),
    );
    expect(mockUseAdditionalModelValidIds).toHaveBeenLastCalledWith({
      modelTypes: ['chat'],
      models: [directModel],
      modelsFetched: true,
      enabled: true,
      ownerTenantId: undefined,
    });

    mockListAllAddedModels.mockResolvedValueOnce({
      data: { code: 0, data: [freshDirectModel] },
    });
    await act(async () => {
      await expect(result.current.resolveValidIds()).resolves.toEqual(
        new Set(['model-2', 'gpt-4o@default@OpenAI', groupRef]),
      );
    });
    expect(mockResolveAdditionalValidIds).toHaveBeenCalledWith([
      freshDirectModel,
    ]);
    queryClient.clear();
  });

  it('rejects a failed direct refresh without resolving additional ids', async () => {
    const directError = new Error('direct refresh failed');
    mockListAllAddedModels
      .mockImplementationOnce(() => new Promise(() => {}))
      .mockRejectedValueOnce(directError);
    const { wrapper, queryClient } = createWrapper();
    const { result } = renderHook(() => useModelValidIds(['chat']), {
      wrapper,
    });

    expect(result.current.isFetched).toBe(false);
    await act(async () => {
      await expect(result.current.resolveValidIds()).rejects.toBe(directError);
    });
    expect(mockResolveAdditionalValidIds).not.toHaveBeenCalled();
    queryClient.clear();
  });

  it.each([
    {
      ownerTenantId: 'owner-1',
      includeAdditionalValidIds: true,
      expectedEnabled: true,
    },
    {
      ownerTenantId: 'tenant-1',
      includeAdditionalValidIds: false,
      expectedEnabled: false,
    },
  ])(
    'forwards owner and explicit opt-out to the additional ID policy: %o',
    async ({ ownerTenantId, includeAdditionalValidIds, expectedEnabled }) => {
      mockAdditional(new Set());
      const { wrapper, queryClient } = createWrapper();
      const { result } = renderHook(
        () =>
          useModelValidIds(
            ['chat'],
            ownerTenantId,
            undefined,
            includeAdditionalValidIds,
          ),
        { wrapper },
      );

      await waitFor(() => {
        expect(result.current.isFetched).toBe(true);
        expect(result.current.isSettled).toBe(true);
      });
      expect(mockUseAdditionalModelValidIds).toHaveBeenLastCalledWith({
        modelTypes: ['chat'],
        models: [directModel],
        modelsFetched: true,
        enabled: expectedEnabled,
        ownerTenantId,
      });
      expect(mockListAllAddedModels).toHaveBeenCalledWith(
        { params: {}, paramsSerializer: { indexes: null } },
        true,
      );
      expect(result.current.validIds).toEqual(
        new Set(['model-1', 'gpt-4@default@OpenAI']),
      );
      queryClient.clear();
    },
  );
});
