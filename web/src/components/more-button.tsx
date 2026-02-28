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
        size={size || 'icon'}
        className={cn(
          'invisible size-3.5 bg-transparent group-hover:bg-transparent',
          'group-focus-within:visible group-hover:visible aria-expanded:visible',
          className,
        )}
        {...props}
      >
        <Ellipsis />
      </Button>
    );
  },
);
