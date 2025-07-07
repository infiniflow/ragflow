import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { Ban, CircleCheck, Trash2 } from 'lucide-react';
import { useCallback } from 'react';

type ICheckboxSetProps = {
  selectAllChunk: (e: any) => void;
  removeChunk: (e?: any) => void;
  switchChunk: (available: number) => void;
  checked: boolean;
};
export default (props: ICheckboxSetProps) => {
  const { selectAllChunk, removeChunk, switchChunk, checked } = props;
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
      <div className="flex items-center gap-3 cursor-pointer">
        <Checkbox
          id="all_chunks_checkbox"
          onCheckedChange={handleSelectAllCheck}
          checked={checked}
          className=" data-[state=checked]:bg-[#1668dc] data-[state=checked]:border-[#1668dc] data-[state=checked]:text-white"
        />
        <Label htmlFor="all_chunks_checkbox">All Chunks</Label>
      </div>
      <div
        className="flex items-center cursor-pointer"
        onClick={handleEnabledClick}
      >
        <CircleCheck size={16} />
        <span className="block ml-1">Enable</span>
      </div>
      <div
        className="flex items-center cursor-pointer"
        onClick={handleDisabledClick}
      >
        <Ban size={16} />
        <span className="block ml-1">Disable</span>
      </div>
      <div
        className="flex items-center text-red-500 cursor-pointer"
        onClick={handleDeleteClick}
      >
        <Trash2 size={16} />
        <span className="block ml-1">Delete</span>
      </div>
    </div>
  );
};
