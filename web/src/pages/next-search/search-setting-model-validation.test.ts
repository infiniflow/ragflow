import { renderHook } from '@testing-library/react';
import { useSearchSettingFormSchema } from './search-setting-hooks';

const mockUseModelValidIds = jest.fn();
let rerankState: Record<string, unknown>;
let llmState: Record<string, unknown>;

jest.mock('@/hooks/use-llm-request', () => ({
  useModelValidIds: (...args: unknown[]) => mockUseModelValidIds(...args),
}));

jest.mock('@/components/model-tree-select', () => ({
  ModelTypeMap: {
    llm_id: ['chat', 'vlm'],
    rerank_id: ['rerank'],
  },
}));

jest.mock('@/components/llm-setting-items/next', () => {
  const { z } = jest.requireActual('zod');
  return {
    LLMIdFormField: { llm_id: z.string().optional() },
    LlmSettingEnabledSchema: {},
    LlmSettingFieldSchema: {},
  };
});

jest.mock('@/components/metadata-filter', () => ({
  MetadataFilterSchema: {},
}));

jest.mock('@/components/rerank-candidates-count-item', () => ({
  rerankCandidatesCountSchema: {},
}));

jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: (key: string) => key }),
}));

const groupRef = 'gp-0123456789abcdef0123456789abcdef';
const formData = {
  name: 'Search',
  search_config: {
    kb_ids: ['dataset-id'],
    keywords_similarity_weight: 0.7,
    web_search: false,
    similarity_threshold: 0.2,
    use_kg: false,
    rerank_id: groupRef,
    use_rerank: true,
    summary: false,
    llm_setting: { llm_id: 'llm-id' },
    related_search: false,
    query_mindmap: false,
    doc_ids: [],
    chat_id: '',
    highlight: false,
    keyword: false,
    chat_settingcross_languages: [],
  },
};

function modelState(validIds: Set<string>, isSettled: boolean) {
  return { validIds, isFetched: true, isSettled };
}

describe('useSearchSettingFormSchema model-group readiness', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    llmState = modelState(new Set(['llm-id']), true);
    mockUseModelValidIds.mockImplementation((modelTypes: string[]) =>
      modelTypes.includes('rerank') ? rerankState : llmState,
    );
  });

  it('does not flag a group while its compatible IDs are still loading', () => {
    rerankState = modelState(new Set(), false);
    const { result, rerender } = renderHook(() => useSearchSettingFormSchema());

    expect(result.current.modelsFetched).toBe(false);
    expect(result.current.formSchema.safeParse(formData).success).toBe(true);

    rerankState = modelState(new Set([groupRef]), true);
    rerender();

    expect(result.current.modelsFetched).toBe(true);
    expect(result.current.formSchema.safeParse(formData).success).toBe(true);
  });

  it('still flags a missing group after validation data has settled', () => {
    rerankState = modelState(new Set(), true);
    const { result } = renderHook(() => useSearchSettingFormSchema());

    const parsed = result.current.formSchema.safeParse(formData);

    expect(parsed.success).toBe(false);
    if (parsed.success) return;
    expect(parsed.error.issues).toContainEqual(
      expect.objectContaining({
        path: ['search_config', 'rerank_id'],
        message: 'common.modelUnavailable',
      }),
    );
  });

  it('falls back to isFetched for the open-source validation state', () => {
    rerankState = { validIds: new Set([groupRef]), isFetched: true };
    llmState = { validIds: new Set(['llm-id']), isFetched: true };
    const { result } = renderHook(() => useSearchSettingFormSchema());

    expect(result.current.modelsFetched).toBe(true);
    expect(result.current.formSchema.safeParse(formData).success).toBe(true);
  });
});
