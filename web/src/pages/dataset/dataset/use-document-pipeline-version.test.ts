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
    expect(result.current?.id).toBe('v1');
    expect(result.current?.alias).toBe('111');
  });

  it('tracks the latest version when nothing is pinned', () => {
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: 'canvas-1',
        pipeline_version_id: '',
      }),
    );

    expect(result.current?.id).toBe('v2');
  });

  it('resolves to undefined when the pinned version is gone', () => {
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: 'canvas-1',
        pipeline_version_id: 'deleted',
      }),
    );

    expect(result.current).toBeUndefined();
  });

  it('stays disabled for built-in parser documents', () => {
    const { result } = renderHook(() =>
      useDocumentPipelineVersion({
        pipeline_id: '',
        pipeline_version_id: '',
      }),
    );

    expect(mockUseFetchCanvasVersionList).toHaveBeenCalledWith(undefined);
    expect(result.current).toBeUndefined();
  });
});
