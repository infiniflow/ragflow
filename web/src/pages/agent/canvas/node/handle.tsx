import { cn } from '@/lib/utils';
import { Handle, HandleProps } from '@xyflow/react';
import { Plus } from 'lucide-react';

export function CommonHandle({ className, ...props }: HandleProps) {
  return (
    <Handle
      {...props}
      className={cn(
        'inline-flex justify-center items-center !bg-background-checked !size-4 !rounded-sm !border-none ',
        className,
      )}
    >
      <Plus className="size-3 pointer-events-none" />
    </Handle>
  );
}
