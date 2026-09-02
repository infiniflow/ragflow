import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { renderHook } from '@testing-library/react';
import { useFetchKnowledgeConfigurationOnMount } from './hooks';

jest.mock('@/hooks/use-knowledge-request', () => ({
  useFetchKnowledgeBaseConfiguration: jest.fn(),
}));
jest.mock('@/hooks/common-hooks', () => ({
  useSetModalState: jest.fn(),
}));
jest.mock('@/hooks/use-user-setting-request', () => ({
  useSelectParserList: jest.fn(() => []),
}));
jest.mock('@/services/knowledge-service', () => ({
  checkEmbedding: jest.fn(),
}));
jest.mock('react-router', () => ({
  useParams: jest.fn(() => ({})),
  useSearchParams: jest.fn(() => [new URLSearchParams()]),
}));

const mockUseFetchKnowledgeBaseConfiguration = jest.mocked(
  useFetchKnowledgeBaseConfiguration,
);

function renderConfigurationHook(parserConfig: Record<string, unknown>) {
  mockUseFetchKnowledgeBaseConfiguration.mockReturnValue({
    data: {
      name: 'Dataset',
      parser_config: parserConfig,
      embedding_model: 'BAAI/bge-large-zh-v1.5',
      chunk_method: 'naive',
    },
    loading: false,
  } as never);

  const reset = jest.fn();
  const form = {
    formState: {
      defaultValues: {
        parser_config: {
          raptor: { use_raptor: true },
          graphrag: { use_graphrag: true },
        },
      },
    },
    reset,
  } as never;

  renderHook(() => useFetchKnowledgeConfigurationOnMount(form));

  return reset.mock.calls[0][0].parser_config;
}

describe('useFetchKnowledgeConfigurationOnMount', () => {
  it('preserves explicitly disabled RAPTOR and GraphRAG settings', () => {
    const parserConfig = renderConfigurationHook({
      raptor: {
        use_raptor: false,
        ext: { clustering_method: 'gmm' },
      },
      graphrag: { use_graphrag: false },
    });

    expect(parserConfig.raptor.use_raptor).toBe(false);
    expect(parserConfig.graphrag.use_graphrag).toBe(false);
  });

  it('keeps form defaults when saved settings omit the enable flags', () => {
    const parserConfig = renderConfigurationHook({
      raptor: { ext: { clustering_method: 'gmm' } },
      graphrag: {},
    });

    expect(parserConfig.raptor.use_raptor).toBe(true);
    expect(parserConfig.graphrag.use_graphrag).toBe(true);
  });
});
