import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { Trash2 } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

type ICheckboxSetProps = {
  selectAllChunk: (e: any) => void;
  removeChunk: (e?: any) => void;
  checked: boolean;
  selectedChunkIds: string[];
};
export default (props: ICheckboxSetProps) => {
  const { selectAllChunk, removeChunk, checked, selectedChunkIds } = props;
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

  const isSelected = useMemo(() => {
    return selectedChunkIds?.length > 0;
  }, [selectedChunkIds]);

  return (
    <div className="flex gap-[40px] py-4 px-2">
      <div className="flex items-center gap-3 cursor-pointer text-muted-foreground hover:text-text-primary">
        <Checkbox
          id="all_chunks_checkbox"
          onCheckedChange={handleSelectAllCheck}
          checked={checked}
          className=" data-[state=checked]:bg-text-primary data-[state=checked]:border-text-primary data-[state=checked]:text-bg-base  border-muted-foreground text-muted-foreground hover:text-bg-base hover:border-text-primary "
        />
        <Label htmlFor="all_chunks_checkbox">{t('chunk.selectAll')}</Label>
      </div>
      {isSelected && (
        <>
          <div
            className="flex items-center cursor-pointer text-red-400 hover:text-red-500"
            onClick={handleDeleteClick}
          >
            <Trash2 size={16} />
            <span className="block ml-1">{t('chunk.delete')}</span>
          </div>
        </>
      )}
    </div>
  );
};
