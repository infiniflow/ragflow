import { forwardRef, HTMLAttributes } from 'react';

import { cn } from '@/lib/utils';

export const BaseNode = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement> & { selected?: boolean }
>(({ className, selected, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'relative rounded bg-card text-card-foreground',
      className,
      selected ? 'border-muted-foreground shadow-lg' : '',
      'hover:ring-1',
    )}
    tabIndex={0}
    {...props}
  />
));

BaseNode.displayName = 'BaseNode';
