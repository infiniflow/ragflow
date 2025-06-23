import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { Ban, CircleCheck, Trash2 } from 'lucide-react';
import { useCallback } from 'react';

export default ({ selectAllChunk, checked }) => {
  const handleSelectAllCheck = useCallback(
    (e: any) => {
      console.log('eee=', e);
      selectAllChunk(e);
    },
    [selectAllChunk],
  );

  return (
    <div className="flex gap-[40px]">
      <div className="flex items-center gap-3 cursor-pointer">
        <Checkbox
          id="all_chunks_checkbox"
          onCheckedChange={handleSelectAllCheck}
          checked={checked}
        />
        <Label htmlFor="all_chunks_checkbox">All Chunks</Label>
      </div>
      <div className="flex items-center cursor-pointer">
        <CircleCheck size={16} />
        <span className="block ml-1">Enable</span>
      </div>
      <div className="flex items-center cursor-pointer">
        <Ban size={16} />
        <span className="block ml-1">Disable</span>
      </div>
      <div className="flex items-center text-red-500 cursor-pointer">
        <Trash2 size={16} />
        <span className="block ml-1">Delete</span>
      </div>
    </div>
  );
};
