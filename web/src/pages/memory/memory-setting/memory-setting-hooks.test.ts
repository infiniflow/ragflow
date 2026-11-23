import { renderHook } from '@testing-library/react';
import { useMemoryFormSchema } from './memory-setting-hooks';

const mockUseModelValidIds = jest.fn();
const mockUseFetchMemoryMessageList = jest.fn();
const mockT = (key: string) => key;
let llmState: Record<string, unknown>;
let embeddingState: Record<string, unknown>;

jest.mock('@/hooks/use-llm-request', () => ({
  useModelValidIds: (...args: unknown[]) => mockUseModelValidIds(...args),
}));

jest.mock('@/components/model-tree-select', () => ({
  ModelTypeMap: {
    llm_id: ['chat', 'vlm'],
    embd_id: ['embedding'],
  },
}));

jest.mock('../memory-message/hook', () => ({
  useFetchMemoryMessageList: () => mockUseFetchMemoryMessageList(),
}));

jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: mockT }),
}));

jest.mock('i18next', () => ({
  ...jest.requireActual('i18next'),
  t: (key: string) => key,
}));

const groupRef = 'gp-0123456789abcdef0123456789abcdef';
const pendingMessage = 'setting.modelGroupSelector.pending';
const baseValue = {
  id: 'memory-1',
  name: 'Memory',
  memory_type: ['raw'],
};

function settledState(validIds: Set<string>) {
  return {
    validIds,
    isFetched: true,
    isSettled: true,
    getPendingMessage: () => undefined,
  };
}

function pendingState(validIds = new Set<string>()) {
  return {
    validIds,
    isFetched: true,
    isSettled: false,
    getPendingMessage: (value: string) =>
      value === groupRef ? pendingMessage : undefined,
  };
}

describe('useMemoryFormSchema', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseFetchMemoryMessageList.mockReturnValue({
      data: { messages: { total_count: 0 } },
    });
    mockUseModelValidIds.mockImplementation((modelTypes: string[]) =>
      modelTypes.includes('embedding') ? embeddingState : llmState,
    );
  });

  it('accepts settled compatible group references for both current user models', () => {
    llmState = settledState(new Set([groupRef]));
    embeddingState = settledState(new Set([groupRef]));
    const { result } = renderHook(() => useMemoryFormSchema());

    const parsed = result.current.formSchema.safeParse({
      ...baseValue,
      llm_id: groupRef,
      embd_id: groupRef,
    });

    expect(parsed.success).toBe(true);
    expect(result.current.modelsFetched).toBe(true);
    expect(mockUseModelValidIds).toHaveBeenCalledWith(
      ['chat', 'vlm'],
      undefined,
      undefined,
    );
    expect(mockUseModelValidIds).toHaveBeenCalledWith(
      ['embedding'],
      undefined,
      undefined,
    );
  });

  it('accepts only direct models in the owner-scoped schema', () => {
    llmState = settledState(new Set(['model-1']));
    embeddingState = settledState(new Set(['model-1']));
    const { result } = renderHook(() => useMemoryFormSchema('tenant-1'));

    const parsed = result.current.formSchema.safeParse({
      ...baseValue,
      llm_id: 'model-1',
      embd_id: 'model-1',
    });

    expect(parsed.success).toBe(true);
    expect(result.current.modelsFetched).toBe(true);
    expect(mockUseModelValidIds).toHaveBeenCalledWith(
      ['chat', 'vlm'],
      'tenant-1',
      undefined,
    );
    expect(mockUseModelValidIds).toHaveBeenCalledWith(
      ['embedding'],
      'tenant-1',
      undefined,
    );
  });

  it('reports pending independently on llm and embedding paths', () => {
    llmState = pendingState();
    embeddingState = pendingState();
    const { result } = renderHook(() => useMemoryFormSchema());

    const parsed = result.current.formSchema.safeParse({
      ...baseValue,
      llm_id: groupRef,
      embd_id: groupRef,
    });
    expect(parsed.success).toBe(false);
    if (parsed.success) throw new Error('Expected schema validation to fail');
    const issues = parsed.error.issues;

    expect(issues).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ path: ['llm_id'], message: pendingMessage }),
        expect.objectContaining({ path: ['embd_id'], message: pendingMessage }),
      ]),
    );
    expect(
      issues.some((issue) => issue.message === 'common.modelUnavailable'),
    ).toBe(false);
    expect(result.current.modelsFetched).toBe(false);
  });

  it('still flags an unavailable direct llm while group readiness is pending', () => {
    llmState = pendingState(new Set(['model-1']));
    embeddingState = pendingState(new Set(['model-1']));
    const { result } = renderHook(() => useMemoryFormSchema());

    const parsed = result.current.formSchema.safeParse({
      ...baseValue,
      llm_id: 'gone',
      embd_id: 'model-1',
    });
    expect(parsed.success).toBe(false);
    if (parsed.success) throw new Error('Expected schema validation to fail');
    const issues = parsed.error.issues;

    expect(issues).toContainEqual(
      expect.objectContaining({
        path: ['llm_id'],
        message: 'common.modelUnavailable',
      }),
    );
    expect(issues.some((issue) => issue.path[0] === 'embd_id')).toBe(false);
  });
});
