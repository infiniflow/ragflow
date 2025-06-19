import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import * as React from 'react';

import { cn } from '@/lib/utils';
import { Loader2, Plus } from 'lucide-react';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:bg-primary/90',
        destructive:
          'bg-destructive text-destructive-foreground hover:bg-destructive/90',
        outline:
          'border border-text-sub-title-invert bg-transparent hover:bg-accent hover:text-accent-foreground',
        secondary:
          'bg-secondary text-secondary-foreground hover:bg-secondary/80',
        ghost: 'hover:bg-accent hover:text-accent-foreground',
        link: 'text-primary underline-offset-4 hover:underline',
        tertiary:
          'bg-colors-background-sentiment-solid-primary text-colors-text-persist-light hover:bg-colors-background-sentiment-solid-primary/80',
        icon: 'bg-colors-background-inverse-standard text-foreground hover:bg-colors-background-inverse-standard/80',
      },
      size: {
        default: 'h-8 px-2.5 py-1.5 ',
        sm: 'h-6 rounded-sm px-2',
        lg: 'h-11 rounded-md px-8',
        icon: 'h-10 w-10',
        auto: 'h-full px-1',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button';
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  },
);
Button.displayName = 'Button';

export const ButtonLoading = React.forwardRef<
  HTMLButtonElement,
  ButtonProps & { loading?: boolean }
>(
  (
    {
      className,
      variant,
      size,
      asChild = false,
      children,
      loading = false,
      disabled,
      ...props
    },
    ref,
  ) => {
    const Comp = asChild ? Slot : 'button';
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
        disabled={loading || disabled}
      >
        {loading && <Loader2 className="animate-spin" />}
        {children}
      </Comp>
    );
  },
);

ButtonLoading.displayName = 'ButtonLoading';

export { Button, buttonVariants };

export const BlockButton = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ children, className, ...props }, ref) => {
    return (
      <Button
        variant={'outline'}
        ref={ref}
        className={cn('w-full border-dashed border-input-border', className)}
        {...props}
      >
        <Plus /> {children}
      </Button>
    );
  },
);
