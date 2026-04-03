import { cn } from '@/lib/utils';
import { Ellipsis } from 'lucide-react';
import React from 'react';
import { Button, ButtonProps } from './ui/button';

export const MoreButton = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, size, ...props }, ref) => {
    return (
      <Button
        ref={ref}
        variant="ghost"
        size={size || 'icon'}
        className={cn(
          'invisible group-hover:visible size-3.5 bg-transparent group-hover:bg-transparent',
          className,
        )}
        {...props}
      >
        <Ellipsis />
      </Button>
    );
  },
);
