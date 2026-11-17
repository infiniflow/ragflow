import { renderHook } from '@testing-library/react';
import { useSearchSettingFormSchema } from './search-setting-hooks';

const mockUseModelValidIds = jest.fn();
const mockT = (key: string) => key;
let rerankState: Record<string, unknown>;
let llmState: Record<string, unknown>;

jest.mock('@/hooks/use-llm-request', () => ({
  useModelValidIds: (...args: unknown[]) => mockUseModelValidIds(...args),
}));

jest.mock('@/components/model-tree-select', () => ({
  ModelTypeMap: {
    llm_id: ['chat', 'vision'],
    rerank_id: ['rerank'],
  },
}));

jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: mockT }),
}));

const groupRef = 'gp-0123456789abcdef0123456789abcdef';
const pendingMessage = 'setting.modelGroupSelector.pending';
const baseValue = {
  name: 'Search',
  search_config: {
    kb_ids: ['kb-1'],
    keywords_similarity_weight: 0.7,
    web_search: false,
    similarity_threshold: 0.2,
    use_kg: false,
    rerank_id: groupRef,
    use_rerank: true,
    rerank_candidates_count: 100,
    summary: true,
    llm_setting: { llm_id: groupRef },
    related_search: false,
    query_mindmap: false,
    doc_ids: [],
    chat_id: '',
    highlight: false,
    keyword: false,
    chat_settingcross_languages: [],
  },
};

function settledState(validIds: Set<string>) {
  return {
    validIds,
    isFetched: true,
    isSettled: true,
    getPendingMessage: () => undefined,
  };
}

describe('useSearchSettingFormSchema', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseModelValidIds.mockImplementation((modelTypes: string[]) =>
      modelTypes.includes('rerank') ? rerankState : llmState,
    );
  });

  it('accepts settled compatible rerank and llm group references', () => {
    rerankState = settledState(new Set([groupRef]));
    llmState = settledState(new Set([groupRef]));
    const { result } = renderHook(() => useSearchSettingFormSchema());

    const parsed = result.current.formSchema.safeParse(baseValue);

    expect(parsed.success).toBe(true);
    expect(result.current.modelsFetched).toBe(true);
  });

  it('combines rerank pending with direct llm unavailability', () => {
    rerankState = {
      validIds: new Set(),
      isFetched: true,
      isSettled: false,
      getPendingMessage: (value: string) =>
        value === groupRef ? pendingMessage : undefined,
    };
    llmState = settledState(new Set(['model-1']));
    const { result } = renderHook(() => useSearchSettingFormSchema());

    const parsed = result.current.formSchema.safeParse({
      ...baseValue,
      search_config: {
        ...baseValue.search_config,
        llm_setting: { llm_id: 'gone' },
      },
    });

    expect(parsed.success).toBe(false);
    if (!parsed.success) {
      expect(parsed.error.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            path: ['search_config', 'rerank_id'],
            message: pendingMessage,
          }),
          expect.objectContaining({
            path: ['search_config', 'llm_setting', 'llm_id'],
            message: 'common.modelUnavailable',
          }),
        ]),
      );
    }
    expect(result.current.modelsFetched).toBe(false);
  });
});
