import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Radio } from '@/components/ui/radio';
import { useDatasetEditButtonDisabled } from '@/ee/hooks/logic-hooks/use-permission';
import { cn } from '@/lib/utils';
import { ListChevronsUpDown, LucideFilter, Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { ChunkTextMode } from '../../constant';
interface ChunkResultBarProps {
  className?: string;
  changeChunkTextMode: (mode: ChunkTextMode) => void;
  textMode: ChunkTextMode;
  total: number;
  available: number | undefined;
  selectAllChunk: (value: boolean) => void;
  handleSetAvailable: (value: number | undefined) => void;
  createChunk: () => void;
  handleInputChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  searchString?: string;
}
export default function ChunkResultBar({
  className,
  changeChunkTextMode,
  textMode,
  total,
  available,
  selectAllChunk,
  handleSetAvailable,
  createChunk,
  handleInputChange,
  searchString,
}: ChunkResultBarProps) {
  const datasetEditButtonDisabled = useDatasetEditButtonDisabled();
  const { t } = useTranslation('translation', { keyPrefix: 'chunk' });
  const handleFilterChange = (e: string | number) => {
    const value = e === -1 ? undefined : (e as number);
    selectAllChunk(false);
    handleSetAvailable(value);
  };
  const filterContent = (
    <div className="w-[200px]">
      <Radio.Group onChange={handleFilterChange} value={available ?? -1}>
        <div className="flex flex-col gap-2 p-4">
          <Radio value={-1}>{t('all')}</Radio>
          <Radio value={1}>{t('enabled')}</Radio>
          <Radio value={0}>{t('disabled')}</Radio>
        </div>
      </Radio.Group>
    </div>
  );

  const handleTextModeChange = (value: string | number) => {
    changeChunkTextMode(value as ChunkTextMode);
  };
  const textModeContent = (
    <Radio.Group onChange={handleTextModeChange} value={textMode}>
      <div className="flex flex-col gap-2 p-4">
        <Radio value={ChunkTextMode.Full}>{t('full')}</Radio>
        <Radio value={ChunkTextMode.Ellipse}>{t('ellipse')}</Radio>
      </div>
    </Radio.Group>
  );

  return (
    <div className={cn('flex justify-end gap-4', className)}>
      <div className="me-auto flex items-center gap-1 text-sm text-text-secondary">
        <span>
          {total} {t('chunks')}
        </span>
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="ghost" size="icon-xs">
              <ListChevronsUpDown />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-40 p-0" align="start">
            {textModeContent}
          </PopoverContent>
        </Popover>
      </div>
      <Popover>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            size="icon"
            disabled={datasetEditButtonDisabled}
          >
            <LucideFilter />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="p-0 w-[200px]">
          {filterContent}
        </PopoverContent>
      </Popover>

      <SearchInput
        className="w-28"
        placeholder={t('search', { keyPrefix: 'common' })}
        onChange={handleInputChange}
        value={searchString}
      />

      <Button
        variant="outline"
        size="icon"
        onClick={() => createChunk()}
        disabled={datasetEditButtonDisabled}
      >
        <Plus size={44} />
      </Button>
    </div>
  );
}
