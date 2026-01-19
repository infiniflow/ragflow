import { Button } from '@/components/ui/button';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { ChunkTextMode } from '../../constant';
interface ChunkResultBarProps {
  changeChunkTextMode: (mode: ChunkTextMode) => void;
  createChunk: (text: string) => void;
  isReadonly: boolean;
}
export default ({
  changeChunkTextMode,
  createChunk,
  isReadonly,
}: ChunkResultBarProps) => {
  const { t } = useTranslate('chunk');
  const [textSelectValue, setTextSelectValue] = useState<ChunkTextMode>(
    ChunkTextMode.Full,
  );
  const textSelectOptions = [
    { label: t(ChunkTextMode.Full), value: ChunkTextMode.Full },
    { label: t(ChunkTextMode.Ellipse), value: ChunkTextMode.Ellipse },
  ];

  const changeTextSelectValue = (value: ChunkTextMode) => {
    setTextSelectValue(value);
    changeChunkTextMode(value);
  };
  return (
    <div className="flex gap-2">
      <div className="flex items-center gap-1 bg-bg-card text-muted-foreground w-fit h-[35px] rounded-md p-1">
        {textSelectOptions.map((option) => (
          <div
            key={option.value}
            className={cn(
              'flex items-center cursor-pointer px-4 py-1 rounded-md',
              {
                'text-primary bg-bg-base': option.value === textSelectValue,
                'text-text-primary': option.value !== textSelectValue,
              },
            )}
            onClick={() => changeTextSelectValue(option.value)}
          >
            {option.label}
          </div>
        ))}
      </div>
      {!isReadonly && (
        <Button
          onClick={() => createChunk('')}
          variant={'secondary'}
          className="bg-bg-card text-muted-foreground hover:bg-card"
        >
          <Plus size={44} />
        </Button>
      )}
    </div>
  );
};
