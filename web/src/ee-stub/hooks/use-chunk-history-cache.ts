export interface ChunkHistoryCache {
  invalidateAfterEdit: () => void;
  invalidateAfterSwitch: () => void;
}

const noop = () => {};

/**
 * Enterprise seam for the chunk caches the shared requests keep in step. Open
 * source keeps no chunk history, so there is no version panel or chunk detail
 * to refresh and every callback does nothing.
 */
export function useChunkHistoryCache(): ChunkHistoryCache {
  return { invalidateAfterEdit: noop, invalidateAfterSwitch: noop };
}
