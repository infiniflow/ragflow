import type { IChunk } from '@/interfaces/database/dataset';

export interface ChunkDetailEntryProps {
  chunk: IChunk;
}

/**
 * Enterprise seam for the chunk card's detail entry. Open source keeps no chunk
 * history, so it records no timestamps and no version for a chunk, and the entry
 * would have nothing to show: leaving the slot empty means the card prints
 * neither the chunk id nor an entry beside it.
 */
export function ChunkDetailEntry(_props: ChunkDetailEntryProps) {
  void _props;
  return null;
}

export default ChunkDetailEntry;
