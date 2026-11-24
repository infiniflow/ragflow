import CopyToClipboard from '@/components/copy-to-clipboard';
import type { IChunk } from '@/interfaces/database/dataset';
import { useTranslation } from 'react-i18next';

export interface ChunkDetailEntryProps {
  chunk: IChunk;
}

/**
 * Enterprise seam for the chunk card's detail entry. Open source keeps no chunk
 * history, so the entry only names the chunk: its id, truncated with the full
 * value in a tooltip, and a button that copies it, so a card can be matched
 * against the `id` the retrieval API returns.
 */
export function ChunkDetailEntry({ chunk }: ChunkDetailEntryProps) {
  const { t } = useTranslation();

  return (
    <span className="flex min-w-0 items-center gap-0.5">
      <span title={chunk.chunk_id} className="truncate font-mono">
        {chunk.chunk_id}
      </span>
      <CopyToClipboard
        text={chunk.chunk_id}
        size="icon-xs"
        variant="ghost"
        className="size-5 shrink-0 text-text-disabled [&_svg]:size-3"
        aria-label={t('chunk.copyChunkId')}
      />
    </span>
  );
}

export default ChunkDetailEntry;
