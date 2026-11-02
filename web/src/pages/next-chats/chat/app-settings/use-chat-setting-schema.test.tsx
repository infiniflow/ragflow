import { renderHook } from '@testing-library/react';
import { WebSearchProvider } from '@/constants/chat';
import { removeUselessFieldsFromValues } from '@/utils/form';
import { hasWebSearchProvider } from '../web-search-api-key';
import { useChatSettingSchema } from './use-chat-setting-schema';

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/llm-setting-items/next', () => ({
  LlmSettingEnabledSchema: {},
  LlmSettingFieldSchema: {},
}));
jest.mock('@/components/metadata-filter', () => ({ MetadataFilterSchema: {} }));
jest.mock('@/components/rerank-candidates-count-item', () => ({
  rerankCandidatesCountSchema: {},
}));
jest.mock('@/components/rerank', () => ({ rerankFormSchema: {} }));
jest.mock('@/components/similarity-slider', () => ({
  similarityThresholdSchema: {},
  keywordsSimilarityWeightSchema: {},
}));
jest.mock('@/components/top-n-item', () => ({ topnSchema: {} }));

const DefaultSettings = {
  name: 'Search chat',
  icon: '',
  dataset_ids: [],
  llm_setting: {},
  prompt_config: {
    system: 'Answer using {knowledge}',
    parameters: [{ key: 'knowledge', optional: false }],
    quote: true,
    keyword: false,
    tts: false,
    refine_multiturn: false,
    web_search_provider: WebSearchProvider.AnySearch,
  },
};

describe('AnySearch settings schema', () => {
  it.each([undefined, '', '   ', 'anysearch-test'])(
    'preserves selected provider and optional key %p through save and reload',
    (key) => {
      const { result } = renderHook(useChatSettingSchema);
      const validated = result.current.parse({
        ...DefaultSettings,
        prompt_config: {
          ...DefaultSettings.prompt_config,
          anysearch_api_key: key,
        },
      });
      const reloaded = JSON.parse(
        JSON.stringify(
          removeUselessFieldsFromValues(validated, 'llm_setting.'),
        ),
      );
      expect(reloaded.prompt_config.web_search_provider).toBe(
        WebSearchProvider.AnySearch,
      );
      expect(reloaded.prompt_config.anysearch_api_key).toBe(key);
      expect(result.current.safeParse(reloaded).success).toBe(true);
      expect(hasWebSearchProvider(reloaded.prompt_config)).toBe(true);
    },
  );

  it('preserves explicit cleared selection and hides Internet after reload', () => {
    const { result } = renderHook(useChatSettingSchema);
    const saved = result.current.parse({
      ...DefaultSettings,
      prompt_config: {
        ...DefaultSettings.prompt_config,
        web_search_provider: '',
        anysearch_api_key: 'retained-key',
        tavily_api_key: 'retained-other-key',
      },
    });
    const reloaded = JSON.parse(JSON.stringify(saved));
    expect(reloaded.prompt_config.web_search_provider).toBe('');
    expect(hasWebSearchProvider(reloaded.prompt_config)).toBe(false);
  });

  it('still rejects a required-key provider without its own key', () => {
    const { result } = renderHook(useChatSettingSchema);
    const saved = result.current.safeParse({
      ...DefaultSettings,
      prompt_config: {
        ...DefaultSettings.prompt_config,
        web_search_provider: WebSearchProvider.Search1API,
        anysearch_api_key: 'other-provider-key',
      },
    });
    expect(saved.success).toBe(false);
    if (!saved.success) {
      expect(saved.error.issues).toContainEqual(
        expect.objectContaining({
          path: ['prompt_config', 'search1api_api_key'],
        }),
      );
    }
  });
});
