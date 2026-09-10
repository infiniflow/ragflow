import { useUpdateKnowledge } from '@/hooks/use-knowledge-request';
import { renderHook } from '@testing-library/react';
import { useSaveDatasetSetting } from './hooks';

let mockIsGoBackend = true;
jest.mock('@/utils/backend-runtime', () => ({
  getBackendLanguage: () => (mockIsGoBackend ? 'go' : 'python'),
}));
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

describe('useSaveDatasetSetting parser_config metadata lift', () => {
  beforeEach(() => {
    mockIsGoBackend = true;
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

  it('makes the extractor metadata group authoritative over the stale top-level copy', async () => {
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
    expect(payload.parser_config.metadata).toEqual({
      enabled: true,
      metadata: [],
      built_in_metadata: [{ key: 'update_time', type: 'time' }],
    });
    expect(
      payload.parser_config['Extractor:AutoExtractDefault'].metadata.enabled,
    ).toBe(true);
  });

  it('leaves the top-level object untouched when the pipeline has no extractor', async () => {
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
    expect(payload.parser_config.metadata).toEqual({
      enabled: true,
      metadata: [],
      built_in_metadata: [],
    });
  });
});
