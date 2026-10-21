import { WebSearchProvider } from '@/constants/chat';
import { ThinkingMode } from '@/hooks/use-thinking-level';
import { buildChatSettingFormSchema } from './use-chat-setting-schema';

const t = (key: string) => key;

const BaseValues = {
  name: 'assistant',
  icon: '',
  dataset_ids: [] as string[],
  prompt_config: {
    quote: true,
    keyword: false,
    tts: false,
    refine_multiturn: true,
    system: 'Answer using {knowledge}',
    parameters: [] as Array<{ key: string; optional: boolean }>,
  },
  rerank_candidates_count: 64,
  similarity_threshold: 0.2,
  keywords_similarity_weight: 0.8,
  llm_setting: {},
  meta_data_filter: { method: 'disabled', manual: [] },
};

function parseInMode(
  mode: ThinkingMode,
  promptConfig: Record<string, unknown> = {},
  overrides: Record<string, unknown> = {},
) {
  const schema = buildChatSettingFormSchema(mode, t);
  return schema.safeParse({
    ...BaseValues,
    ...overrides,
    prompt_config: { ...BaseValues.prompt_config, ...promptConfig },
  });
}

describe('buildChatSettingFormSchema', () => {
  it('rejects a blank system prompt in naive mode', () => {
    const result = parseInMode(ThinkingMode.Naive, { system: '' });
    expect(result.success).toBe(false);
    expect(result.error?.issues).toContainEqual(
      expect.objectContaining({
        path: ['prompt_config', 'system'],
        message: 'systemMessage',
      }),
    );
  });

  it('accepts a blank system prompt in agentic mode even with datasets selected', () => {
    const result = parseInMode(
      ThinkingMode.Agentic,
      { system: '' },
      { dataset_ids: ['kb1'] },
    );
    expect(result.success).toBe(true);
  });

  it('still flags a system prompt missing {knowledge} in naive mode', () => {
    const result = parseInMode(
      ThinkingMode.Naive,
      { system: 'answer the question' },
      { dataset_ids: ['kb1'] },
    );
    expect(result.success).toBe(false);
    expect(result.error?.issues).toContainEqual(
      expect.objectContaining({
        path: ['prompt_config', 'system'],
        message: 'knowledgePlaceholderMessage',
      }),
    );
  });

  it('relaxes the variable key rule in agentic mode only', () => {
    const parameters = [{ key: '', optional: true }];
    expect(parseInMode(ThinkingMode.Agentic, { parameters }).success).toBe(
      true,
    );

    const naiveResult = parseInMode(ThinkingMode.Naive, { parameters });
    expect(naiveResult.success).toBe(false);
    expect(naiveResult.error?.issues).toContainEqual(
      expect.objectContaining({
        path: ['prompt_config', 'parameters', 0, 'key'],
        message: 'variableKeyMessage',
      }),
    );
  });

  it.each([ThinkingMode.Naive, ThinkingMode.Agentic])(
    'requires the web search API key in %s mode',
    (mode) => {
      const result = parseInMode(mode, {
        web_search_provider: WebSearchProvider.Tavily,
      });
      expect(result.success).toBe(false);
      expect(result.error?.issues).toContainEqual(
        expect.objectContaining({
          path: ['prompt_config', 'tavily_api_key'],
          message: 'webSearchApiKeyRequired',
        }),
      );
    },
  );

  it('keeps agentic_rag through parsing instead of stripping it', () => {
    const agenticRag = {
      content: 'custom template',
      tools: ['grep_chunks', 'list_chunks'],
    };
    const result = parseInMode(ThinkingMode.Agentic, {
      agentic_rag: agenticRag,
    });
    expect(result.success).toBe(true);
    expect(result.data?.prompt_config.agentic_rag).toEqual(agenticRag);
  });

  it('rejects agentic templates longer than 32768 characters', () => {
    const result = parseInMode(ThinkingMode.Agentic, {
      agentic_rag: { content: 'x'.repeat(32769) },
    });
    expect(result.success).toBe(false);
    expect(result.error?.issues).toContainEqual(
      expect.objectContaining({
        path: ['prompt_config', 'agentic_rag', 'content'],
        message: 'agenticTemplateTooLong',
      }),
    );
  });
});
