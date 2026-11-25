export interface ChunkIdQuery {
  /**
   * Complete ids, matched exactly.
   */
  ids: string[];
}

/**
 * Enterprise seam for the chunk id search. Open source searches the chunk text
 * only, so the box never reads as an id lookup and the parsed query is always
 * absent.
 */
export function useChunkIdQuery(_search?: string): ChunkIdQuery | undefined {
  return undefined;
}
