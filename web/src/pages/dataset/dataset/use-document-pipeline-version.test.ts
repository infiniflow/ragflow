import { renderHook } from '@testing-library/react';

const mockUseFetchCanvasVersionList = jest.fn();

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchCanvasVersionList: (canvasId?: string) =>
    mockUseFetchCanvasVersionList(canvasId),
}));

import { useDocumentPipelineVersion } from './use-document-pipeline-version';

const versions = [
  { id: 'v2', title: 'newest', created_at: '2026-10-29' },
  { id: 'v1', title: 'older', created_at: '2026-10-28', alias: '111' },
];

describe('useDocumentPipelineVersion', () => {
  beforeEach(() => {
    mockUseFetchCanvasVersionList.mockReset();
    // Mirrors useFetchCanvasVersionList: a disabled query (no canvas id)
    // never produces data.
    mockUseFetchCanvasVersionList.mockImplementation((canvasId?: string) => ({
      data: canvasId ? versions : undefined,
      loading: false,
      isError: false,
    }));
  });

  it('returns the pinned version', () => {
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: 'canvas-1',
        pipeline_version_id: 'v1',
      }),
    );

    expect(mockUseFetchCanvasVersionList).toHaveBeenCalledWith('canvas-1');
    expect(result.current.version?.id).toBe('v1');
    expect(result.current.version?.alias).toBe('111');
    expect(result.current.missing).toBe(false);
  });

  it('tracks the latest version when nothing is pinned', () => {
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: 'canvas-1',
        pipeline_version_id: '',
      }),
    );

    expect(result.current.version?.id).toBe('v2');
    expect(result.current.missing).toBe(false);
  });

  it('flags a pinned version that no longer exists as missing', () => {
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: 'canvas-1',
        pipeline_version_id: 'deleted',
      }),
    );

    expect(result.current.version).toBeUndefined();
    expect(result.current.missing).toBe(true);
  });

  it('flags an unpinned binding whose pipeline has no published versions left', () => {
    mockUseFetchCanvasVersionList.mockReturnValue({
      data: [],
      loading: false,
      isError: false,
    });
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: 'canvas-1',
        pipeline_version_id: '',
      }),
    );

    expect(result.current.version).toBeUndefined();
    expect(result.current.missing).toBe(true);
  });

  it('flags a pipeline whose version list failed to load (deleted pipeline)', () => {
    mockUseFetchCanvasVersionList.mockReturnValue({
      data: undefined,
      loading: false,
      isError: true,
    });
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: 'deleted-canvas',
        pipeline_version_id: 'v1',
      }),
    );

    expect(result.current.missing).toBe(true);
  });

  it('stays quiet while the version list is in flight', () => {
    mockUseFetchCanvasVersionList.mockReturnValue({
      data: undefined,
      loading: true,
      isError: false,
    });
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: 'canvas-1',
        pipeline_version_id: 'v1',
      }),
    );

    expect(result.current.version).toBeUndefined();
    expect(result.current.missing).toBe(false);
  });

  it('stays disabled for built-in parser documents', () => {
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: '',
        pipeline_version_id: '',
      }),
    );

    expect(mockUseFetchCanvasVersionList).toHaveBeenCalledWith(undefined);
    expect(result.current.version).toBeUndefined();
    expect(result.current.missing).toBe(false);
  });
});
