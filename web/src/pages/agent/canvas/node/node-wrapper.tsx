import { cn } from '@/lib/utils';
import { HTMLAttributes, PropsWithChildren } from 'react';

export function NodeWrapper({
  children,
  className,
}: PropsWithChildren & HTMLAttributes<HTMLDivElement>) {
  return (
    <section
      className={cn(
        'bg-background-header-bar p-2.5 rounded-md w-[200px] text-xs',
        className,
      )}
    >
      {children}
    </section>
  );
}
