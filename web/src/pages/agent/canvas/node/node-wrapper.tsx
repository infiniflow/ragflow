import { cn } from '@/lib/utils';
import { HTMLAttributes } from 'react';

type IProps = HTMLAttributes<HTMLDivElement> & { selected?: boolean };

export function NodeWrapper({ children, className, selected }: IProps) {
  return (
    <section
      className={cn(
        'bg-text-title-invert p-2.5 rounded-md w-[200px] text-xs',
        { 'border border-background-checked': selected },
        className,
      )}
    >
      {children}
    </section>
  );
}
