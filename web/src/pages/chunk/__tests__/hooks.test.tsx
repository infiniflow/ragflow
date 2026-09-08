import { act, renderHook } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { useHandleChunkCardClick, useTargetChunkFromQuery } from '../hooks';

// The real modules drag the whole request stack (axios, SSE streams) into
// jsdom; neither hook under test touches them.
jest.mock('@/hooks/use-chunk-request', () => ({
  useCreateChunk: jest.fn(),
  useDeleteChunk: jest.fn(),
  useSelectChunkList: jest.fn(),
}));

jest.mock('@/hooks/common-hooks', () => ({
  useSetModalState: jest.fn(),
  useShowDeleteConfirm: jest.fn(),
}));

jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: jest.fn(),
}));

function wrapperFor(initialEntry: string) {
  return function Wrapper({ children }: { children?: React.ReactNode }) {
    return <MemoryRouter initialEntries={[initialEntry]}>{children}</MemoryRouter>;
  };
}

function useTargetChunkHarness() {
  const target = useTargetChunkFromQuery();
  const { search } = useLocation();
  return { ...target, search };
}

describe('useTargetChunkFromQuery', () => {
  it('reads the chunk id handed over by the retrieval testing page', () => {
    const { result } = renderHook(useTargetChunkHarness, {
      wrapper: wrapperFor('/chunk/parsed/chunks?id=kb-1&doc_id=doc-1&chunk_id=c-9'),
    });

    expect(result.current.targetChunkId).toBe('c-9');
  });

  it('reports no target when the param is absent', () => {
    const { result } = renderHook(useTargetChunkHarness, {
      wrapper: wrapperFor('/chunk/parsed/chunks?id=kb-1&doc_id=doc-1'),
    });

    expect(result.current.targetChunkId).toBe('');
  });

  it('drops the chunk id and rewinds to page one when cleared', () => {
    const { result } = renderHook(useTargetChunkHarness, {
      wrapper: wrapperFor(
        '/chunk/parsed/chunks?id=kb-1&doc_id=doc-1&chunk_id=c-9&page=4',
      ),
    });

    act(() => {
      result.current.clearTargetChunkId();
    });

    const params = new URLSearchParams(result.current.search);
    expect(params.get('chunk_id')).toBeNull();
    expect(params.get('page')).toBe('1');
    expect(params.get('id')).toBe('kb-1');
    expect(params.get('doc_id')).toBe('doc-1');
    expect(result.current.targetChunkId).toBe('');
  });
});

describe('useHandleChunkCardClick', () => {
  it('starts with the incoming chunk preselected', () => {
    const { result } = renderHook(() => useHandleChunkCardClick('c-9'));

    expect(result.current.selectedChunkId).toBe('c-9');
  });

  it('starts with no selection when no chunk is handed over', () => {
    const { result } = renderHook(() => useHandleChunkCardClick());

    expect(result.current.selectedChunkId).toBe('');
  });

  it('replaces the preselection when another card is clicked', () => {
    const { result } = renderHook(() => useHandleChunkCardClick('c-9'));

    act(() => {
      result.current.handleChunkCardClick('c-3');
    });

    expect(result.current.selectedChunkId).toBe('c-3');
  });
});
