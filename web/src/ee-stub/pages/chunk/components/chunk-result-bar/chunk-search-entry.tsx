import { SearchInput } from '@/components/ui/input';
import { cn } from '@/lib/utils';

export interface ChunkSearchEntryProps {
  className?: string;
  searchString?: string;
  handleInputChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

/**
 * Enterprise seam for the chunk list's search box. Open source searches the
 * chunk text only, so this slot keeps the plain box it has always had: no mode
 * switch, and the search placeholder that comes with the input.
 */
export function ChunkSearchEntry({
  className,
  searchString,
  handleInputChange,
}: ChunkSearchEntryProps) {
  return (
    <SearchInput
      className={cn('w-28', className)}
      onChange={handleInputChange}
      value={searchString}
    />
  );
}
