import { cn } from '@/lib/utils';
import { HTMLAttributes } from 'react';

type IProps = HTMLAttributes<HTMLDivElement> & { selected?: boolean };

export function NodeWrapper({ children, className, selected }: IProps) {
  return (
    <section
      className={cn(
        'bg-text-title-invert p-2.5 rounded-sm w-[200px] text-xs',
        { 'border border-accent-primary': selected },
        className,
      )}
    >
      {children}
    </section>
  );
}
