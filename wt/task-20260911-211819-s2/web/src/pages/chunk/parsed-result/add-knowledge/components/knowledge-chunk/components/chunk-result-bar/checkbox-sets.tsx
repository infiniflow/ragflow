import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import {
  LucideToggleLeft,
  LucideToggleRight,
  LucideTrash2,
} from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

type ICheckboxSetProps = {
  className?: string;
  selectAllChunk: (e: any) => void;
  removeChunk: (e?: any) => void;
  switchChunk: (available: number) => void;
  checked: boolean;
  selectedChunkIds: string[];
};
export default function CheckboxSets(props: ICheckboxSetProps) {
  const {
    className,
    selectAllChunk,
    removeChunk,
    switchChunk,
    checked,
    selectedChunkIds,
  } = props;
  const { t } = useTranslation();
  const handleSelectAllCheck = useCallback(
    (e: any) => {
      console.log('eee=', e);
      selectAllChunk(e);
    },
    [selectAllChunk],
  );

  const handleDeleteClick = useCallback(() => {
    removeChunk();
  }, [removeChunk]);

  const handleEnabledClick = useCallback(() => {
    switchChunk(1);
  }, [switchChunk]);

  const handleDisabledClick = useCallback(() => {
    switchChunk(0);
  }, [switchChunk]);

  const isSelected = useMemo(() => {
    return selectedChunkIds?.length > 0;
  }, [selectedChunkIds]);

  return (
    <div className={cn('flex gap-4 text-sm text-text-secondary', className)}>
      <Label className="flex items-center gap-2">
        <Checkbox onCheckedChange={handleSelectAllCheck} checked={checked} />
        <span>{t('chunk.selectAll')}</span>
      </Label>

      {isSelected && (
        <>
          <Button variant="outline" onClick={handleEnabledClick}>
            <LucideToggleRight size={16} />
            {t('chunk.enable')}
          </Button>

          <Button variant="outline" onClick={handleDisabledClick}>
            <LucideToggleLeft size={16} />
            <span className="block ml-1">{t('chunk.disable')}</span>
          </Button>

          <Button variant="danger" onClick={handleDeleteClick}>
            <LucideTrash2 className="size-[1em]" />
            {t('chunk.delete')}
          </Button>
        </>
      )}
    </div>
  );
}
