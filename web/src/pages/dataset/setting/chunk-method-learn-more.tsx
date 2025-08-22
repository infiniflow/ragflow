import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { X } from 'lucide-react';
import { useState } from 'react';
import CategoryPanel from './category-panel';

export default ({
  tab = 'generalForm',
  parserId,
}: {
  tab: 'generalForm' | 'chunkMethodForm';
  parserId: string;
}) => {
  const [visible, setVisible] = useState(false);

  return (
    <div
      className={cn('hidden flex-1', {
        'flex flex-col': tab === 'chunkMethodForm',
      })}
    >
      <div>
        <Button
          variant="outline"
          onClick={() => {
            setVisible(!visible);
          }}
        >
          Learn More
        </Button>
      </div>
      <div
        className="bg-[#FFF]/10 p-[20px] rounded-[12px] mt-[10px] relative flex-1 overflow-auto"
        style={{ display: visible ? 'block' : 'none' }}
      >
        <CategoryPanel chunkMethod={parserId}></CategoryPanel>
        <div
          className="absolute right-1 top-1 cursor-pointer hover:text-[#FFF]/30"
          onClick={() => {
            setVisible(false);
          }}
        >
          <X />
        </div>
      </div>
    </div>
  );
};
