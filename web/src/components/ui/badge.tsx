import { cva, type VariantProps } from 'class-variance-authority';
import * as React from 'react';

import { cn } from '@/lib/utils';

const badgeVariants = cva(
  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors outline-none focus:outline-none',
  {
    variants: {
      variant: {
        default:
          'border-transparent bg-bg-primary text-text-primary hover:bg-primary/80',
        secondary:
          'border-transparent bg-bg-card text-text-secondary rounded-md',
        success:
          'border-transparent bg-state-success/5 text-state-success rounded-md',
        destructive:
          'border-transparent bg-state-error/5 text-state-error rounded-md',
        outline: 'text-foreground',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { Badge, badgeVariants };
