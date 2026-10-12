import { useUpdateKnowledge } from '@/hooks/use-knowledge-request';
import { renderHook } from '@testing-library/react';
import { useSaveDatasetSetting } from './hooks';

jest.mock('@/hooks/use-knowledge-request', () => ({
  useFetchDatasetPipelineConfiguration: jest.fn(),
  useUpdateKnowledge: jest.fn(),
}));
jest.mock('@/pages/user-setting/data-source/constant', () => ({
  useDataSourceInfo: jest.fn(() => ({ dataSourceInfo: {} })),
}));
jest.mock('@/services/knowledge-service', () => ({
  checkEmbedding: jest.fn(),
}));

const mockUseUpdateKnowledge = jest.mocked(useUpdateKnowledge);

describe('useSaveDatasetSetting parser_config metadata scoping', () => {
  beforeEach(() => {
    mockUseUpdateKnowledge.mockReset();
  });

  function setup() {
    const saveKnowledgeConfiguration = jest.fn();
    mockUseUpdateKnowledge.mockReturnValue({
      saveKnowledgeConfiguration,
      loading: false,
    } as never);
    const {
      result: { current },
    } = renderHook(() => useSaveDatasetSetting());
    return { handleSave: current.handleSave, saveKnowledgeConfiguration };
  }

  it('keeps the extractor node metadata authoritative and emits no flat metadata key', async () => {
    const { handleSave, saveKnowledgeConfiguration } = setup();

    await handleSave({
      parse_type: 'built-in',
      parser_config: {
        'Extractor:AutoExtractDefault': {
          llm_id: 'llm-a',
          metadata: {
            enabled: true,
            metadata: [],
            built_in_metadata: [{ key: 'update_time', type: 'time' }],
          },
        },
        metadata: {
          enabled: false,
          metadata: [],
          built_in_metadata: [],
        },
      },
    } as never);

    const payload = saveKnowledgeConfiguration.mock.calls[0][0];
    // The Go backend requires component-scoped keys: no flat `metadata` transport
    // key is emitted, and the extractor node's own metadata wins over any stale
    // top-level copy present in the form payload.
    expect(payload.parser_config).not.toHaveProperty('metadata');
    expect(
      payload.parser_config['Extractor:AutoExtractDefault'].metadata,
    ).toEqual({
      enabled: true,
      metadata: [],
      built_in_metadata: [{ key: 'update_time', type: 'time' }],
    });
  });

  it('does not emit a flat metadata key when the pipeline has no extractor', async () => {
    const { handleSave, saveKnowledgeConfiguration } = setup();

    await handleSave({
      parse_type: 'built-in',
      parser_config: {
        'Tokenizer:SomeNode': { fields: 'text' },
        metadata: {
          enabled: true,
          metadata: [],
          built_in_metadata: [],
        },
      },
    } as never);

    const payload = saveKnowledgeConfiguration.mock.calls[0][0];
    expect(payload.parser_config).not.toHaveProperty('metadata');
    expect(payload.parser_config['Tokenizer:SomeNode']).toEqual({
      fields: 'text',
    });
  });
});
