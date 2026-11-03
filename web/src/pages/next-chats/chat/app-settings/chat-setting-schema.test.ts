import { WebSearchProvider } from '@/constants/chat';
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

function parseSettings(
  promptConfig: Record<string, unknown> = {},
  overrides: Record<string, unknown> = {},
) {
  const schema = buildChatSettingFormSchema(t);
  return schema.safeParse({
    ...BaseValues,
    ...overrides,
    prompt_config: { ...BaseValues.prompt_config, ...promptConfig },
  });
}

describe('buildChatSettingFormSchema', () => {
  it('rejects a blank system prompt', () => {
    const result = parseSettings({ system: '' });
    expect(result.success).toBe(false);
    expect(result.error?.issues).toContainEqual(
      expect.objectContaining({
        path: ['prompt_config', 'system'],
        message: 'systemMessage',
      }),
    );
  });

  it('flags a system prompt missing {knowledge} when datasets are selected', () => {
    const result = parseSettings(
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

  it('rejects blank variable keys', () => {
    const parameters = [{ key: '', optional: true }];
    const result = parseSettings({ parameters });
    expect(result.success).toBe(false);
    expect(result.error?.issues).toContainEqual(
      expect.objectContaining({
        path: ['prompt_config', 'parameters', 0, 'key'],
        message: 'variableKeyMessage',
      }),
    );
  });

  it('requires the web search API key', () => {
    const result = parseSettings({
      web_search_provider: WebSearchProvider.Tavily,
    });
    expect(result.success).toBe(false);
    expect(result.error?.issues).toContainEqual(
      expect.objectContaining({
        path: ['prompt_config', 'tavily_api_key'],
        message: 'webSearchApiKeyRequired',
      }),
    );
  });

  it('keeps agentic_rag through parsing instead of stripping it', () => {
    const agenticRag = {
      content: 'custom template',
      tools: ['grep_chunks', 'list_chunks'],
    };
    const result = parseSettings({ agentic_rag: agenticRag });
    expect(result.success).toBe(true);
    expect(result.data?.prompt_config.agentic_rag).toEqual(agenticRag);
  });

  it('rejects agentic templates longer than 32768 characters', () => {
    const result = parseSettings({
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
