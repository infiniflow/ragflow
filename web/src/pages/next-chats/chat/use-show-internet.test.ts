import { renderHook } from '@testing-library/react';
import { WebSearchProvider } from '@/constants/chat';
import { useShowInternet } from './use-show-internet';

const mockFetchChat = jest.fn();
jest.mock('@/hooks/use-chat-request', () => ({
  useFetchChat: () => mockFetchChat(),
}));

describe('saved provider Internet visibility', () => {
  it('follows reloaded keyless AnySearch settings and a cleared selection', () => {
    mockFetchChat.mockReturnValue({
      data: {
        prompt_config: {
          web_search_provider: WebSearchProvider.AnySearch,
          anysearch_api_key: '',
        },
      },
    });
    const { result, rerender } = renderHook(useShowInternet);
    expect(result.current).toBe(true);

    mockFetchChat.mockReturnValue({
      data: {
        prompt_config: {
          web_search_provider: '',
          anysearch_api_key: 'retained-key',
          tavily_api_key: 'retained-other-key',
        },
      },
    });
    rerender();
    expect(result.current).toBe(false);
  });

  it('hides Internet until saved settings arrive', () => {
    mockFetchChat.mockReturnValue({ data: undefined });
    expect(renderHook(useShowInternet).result.current).toBe(false);
  });
});
