import { renderHook } from '@testing-library/react';

const mockUseFetchCanvasVersionList = jest.fn();

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchCanvasVersionList: (canvasId?: string) =>
    mockUseFetchCanvasVersionList(canvasId),
}));

import { useDatasetPipelineMissing } from './use-dataset-pipeline-missing';

const versions = [{ id: 'v1', title: 'newest', created_at: '2026-10-29' }];

describe('useDatasetPipelineMissing', () => {
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

  it('stays quiet before the dataset configuration has loaded', () => {
    const { result } = renderHook(() => useDatasetPipelineMissing(undefined));

    expect(mockUseFetchCanvasVersionList).toHaveBeenCalledWith(undefined);
    expect(result.current).toBe(false);
  });

  it('stays disabled for built-in parser datasets', () => {
    const { result } = renderHook(() => useDatasetPipelineMissing(''));

    expect(mockUseFetchCanvasVersionList).toHaveBeenCalledWith('');
    expect(result.current).toBe(false);
  });

  it('stays quiet while the version list is in flight', () => {
    mockUseFetchCanvasVersionList.mockReturnValue({
      data: undefined,
      loading: true,
      isError: false,
    });
    const { result } = renderHook(() => useDatasetPipelineMissing('canvas-1'));

    expect(result.current).toBe(false);
  });

  it('flags a pipeline whose version list failed to load (deleted pipeline)', () => {
    mockUseFetchCanvasVersionList.mockReturnValue({
      data: undefined,
      loading: false,
      isError: true,
    });
    const { result } = renderHook(() =>
      useDatasetPipelineMissing('deleted-canvas'),
    );

    expect(result.current).toBe(true);
  });

  it('flags a pipeline with no published versions left', () => {
    mockUseFetchCanvasVersionList.mockReturnValue({
      data: [],
      loading: false,
      isError: false,
    });
    const { result } = renderHook(() => useDatasetPipelineMissing('canvas-1'));

    expect(result.current).toBe(true);
  });

  it('stays available while any published version exists', () => {
    const { result } = renderHook(() => useDatasetPipelineMissing('canvas-1'));

    expect(result.current).toBe(false);
  });
});
