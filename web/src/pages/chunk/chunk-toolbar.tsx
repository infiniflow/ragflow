import { Button } from '@/components/ui/button';
import { Copy } from 'lucide-react';

interface ChunkToolbarProps {
  text: string;
}

export function ChunkToolbar({ text }: ChunkToolbarProps) {
  return (
    <div className="flex justify-between px-9">
      <span className="text-colors-text-neutral-strong text-3xl font-bold">
        {text}
      </span>
      <div className="flex items-center gap-3">
        <Button variant={'icon'} size={'icon'}>
          <Copy />
        </Button>
        <Button variant={'outline'} size={'sm'}>
          Export
        </Button>
      </div>
    </div>
  );
}
