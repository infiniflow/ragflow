import { cn } from '@/lib/utils';
import { HTMLAttributes } from 'react';

type IProps = HTMLAttributes<HTMLDivElement> & { selected?: boolean };

export function NodeWrapper({ children, className, selected }: IProps) {
  return (
    <section
      className={cn(
        'bg-bg-component p-2.5 rounded-md w-[200px] border border-border-button text-xs group hover:shadow-md',
        { 'border border-accent-primary': selected },
        className,
      )}
    >
      {children}
    </section>
  );
}
