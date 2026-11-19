import { LLMFactory } from '@/constants/llm';
import llmService from '@/services/llm-service';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { useListProviderModels } from '../use-llm-request';

jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: {
    listProviderModels: jest.fn().mockResolvedValue({
      data: { code: 0, data: [] },
    }),
  },
}));

jest.mock('../use-warn-empty-model', () => ({
  useWarnEmptyModel: jest.fn(),
}));

describe('XunFei Retrieval model listing', () => {
  it.each([
    { api_key: 'test-key' },
    { use_saved_credentials: true, instance_name: 'saved-instance' },
  ])(
    'fetches the catalog without sending instance credentials',
    async (credentials) => {
      const queryClient = new QueryClient();
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      );
      const { result } = renderHook(() => useListProviderModels(), { wrapper });

      await act(async () => {
        await result.current.listProviderModels({
          provider_name: LLMFactory.XunFeiRetrieval,
          base_url: 'https://maas-api.cn-huabei-1.xf-yun.com/v2',
          ...credentials,
        });
      });

      expect(llmService.listProviderModels).toHaveBeenLastCalledWith(
        { provider_name: LLMFactory.XunFeiRetrieval, params: {} },
        true,
      );
    },
  );
});
