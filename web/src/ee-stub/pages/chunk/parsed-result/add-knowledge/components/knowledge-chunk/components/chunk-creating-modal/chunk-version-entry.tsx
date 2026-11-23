interface ChunkVersionEntryProps {
  chunkId: string;
  docId: string;
}

/**
 * Enterprise seam for the chunk card's history entry. Open source keeps no chunk
 * history, so the card offers no entry and the action column keeps just edit and
 * delete.
 */
export function ChunkVersionEntry(_props: ChunkVersionEntryProps) {
  void _props;
  return null;
}

export default ChunkVersionEntry;
