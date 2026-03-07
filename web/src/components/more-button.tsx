import { cn } from '@/lib/utils';
import { Ellipsis } from 'lucide-react';
import React from 'react';
import { Button, ButtonProps } from './ui/button';

export const MoreButton = React.forwardRef<HTMLButtonElement, ButtonProps>(
  function MoreButton({ className, size, ...props }, ref) {
    return (
      <Button
        ref={ref}
        variant="ghost"
        size={size || 'icon-xs'}
        className={cn(
          'opacity-0 size-3.5 transition-all bg-transparent group-hover:bg-transparent',
          'group-focus-within:opacity-100 group-hover:opacity-100 aria-expanded:opacity-100',
          className,
        )}
        {...props}
      >
        <Ellipsis />
      </Button>
    );
  },
);
