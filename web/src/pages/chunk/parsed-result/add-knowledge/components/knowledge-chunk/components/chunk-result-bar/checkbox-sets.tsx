import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { Ban, CircleCheck, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

type ICheckboxSetProps = {
  selectAllChunk: (e: any) => void;
  removeChunk: (e?: any) => void;
  switchChunk: (available: number) => void;
  checked: boolean;
};
export default (props: ICheckboxSetProps) => {
  const { selectAllChunk, removeChunk, switchChunk, checked } = props;
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

  return (
    <div className="flex gap-[40px] p-4">
      <div className="flex items-center gap-3 cursor-pointer text-muted-foreground hover:text-white">
        <Checkbox
          id="all_chunks_checkbox"
          onCheckedChange={handleSelectAllCheck}
          checked={checked}
          className=" data-[state=checked]:bg-white data-[state=checked]:border-white data-[state=checked]:text-black  border-muted-foreground text-muted-foreground hover:text-black hover:border-white "
        />
        <Label htmlFor="all_chunks_checkbox">{t('chunk.selectAll')}</Label>
      </div>
      <div
        className="flex items-center cursor-pointer text-muted-foreground hover:text-white"
        onClick={handleEnabledClick}
      >
        <CircleCheck size={16} />
        <span className="block ml-1">{t('chunk.enable')}</span>
      </div>
      <div
        className="flex items-center cursor-pointer text-muted-foreground hover:text-white"
        onClick={handleDisabledClick}
      >
        <Ban size={16} />
        <span className="block ml-1">{t('chunk.disable')}</span>
      </div>
      <div
        className="flex items-center cursor-pointer text-red-400 hover:text-red-500"
        onClick={handleDeleteClick}
      >
        <Trash2 size={16} />
        <span className="block ml-1">{t('chunk.delete')}</span>
      </div>
    </div>
  );
};
