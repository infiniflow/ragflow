import type { IChunk } from '@/interfaces/database/dataset';

export interface ChunkVersionBadgeProps {
  chunk: IChunk;
}

/**
 * Enterprise seam for the chunk card's version badge. Open source keeps no chunk
 * history, so no chunk carries a version and the strip keeps just the chunk type.
 */
export function ChunkVersionBadge(_props: ChunkVersionBadgeProps) {
  void _props;
  return null;
}

export default ChunkVersionBadge;
