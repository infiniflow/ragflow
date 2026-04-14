import { cn } from '@/lib/utils';
import React from 'react';

interface SpinProps {
  spinning?: boolean;
  size?: 'small' | 'default' | 'large';
  className?: string;
  children?: React.ReactNode;
}

const sizeClasses = {
  small: 'w-4 h-4',
  default: 'w-6 h-6',
  large: 'w-8 h-8',
};

const minSizeClasses = {
  small: 'min-w-4 min-h-4',
  default: 'min-w-6 min-h-6',
  large: 'min-w-8 min-h-8',
};

export const Spin: React.FC<SpinProps> = ({
  spinning = true,
  size = 'default',
  className,
  children,
}) => {
  return (
    <div
      className={cn(
        'relative',
        {
          'after:content-[""] after:absolute after:inset-0 after:z-10 after:bg-text-primary/40 after:transition-all after:duration-300 h-full w-full':
            spinning,
        },
        className,
      )}
    >
      {spinning && (
        <div
          className={cn(
            'absolute inset-0 z-10 flex items-center justify-center bg-text-primary/30',
            minSizeClasses[size],
          )}
        >
          <div
            className={cn(
              'rounded-full border-muted-foreground border-2 border-t-transparent animate-spin',
              sizeClasses[size],
            )}
          ></div>
        </div>
      )}
      {children}
    </div>
  );
};
