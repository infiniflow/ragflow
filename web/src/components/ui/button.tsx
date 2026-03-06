import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import * as React from 'react';

import { cn } from '@/lib/utils';
import { LucideLoader2, Plus } from 'lucide-react';
import { Link, LinkProps } from 'react-router';

const buttonVariants = cva(
  cn(
    'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors outline-0',
    'disabled:pointer-events-none disabled:opacity-50 rounded border-0.5 border-transparent',
    '[&_svg]:pointer-events-none [&_svg:not([class*="size-"])]:size-4 shrink-0 [&_svg]:shrink-0',
  ),
  {
    variants: {
      variant: {
        // Solid variant series:
        // Button has its own background color, may have borders
        default:
          'bg-text-primary text-bg-base shadow-xs hover:bg-text-primary/90 focus-visible:bg-text-primary/90',

        secondary: `
          bg-bg-card
          hover:text-text-primary hover:bg-border-button
          focus-visible:text-text-primary focus-visible:bg-border-button
        `,

        highlighted: `
          bg-text-primary text-bg-base border-b-4 border-b-accent-primary
          hover:bg-text-primary/90 focus-visible:bg-text-primary/90
        `,

        accent: `
          bg-accent-primary text-white
          hover:bg-accent-primary/90 focus-visible:bg-accent-primary/90
        `,

        destructive: `
          bg-state-error text-white shadow-xs
          hover:bg-state-error/90 focus-visible:ring-state-error/20 dark:focus-visible:ring-state-error/40
        `,

        // Outline variant series
        // Button has transparent or greyish background, may have borders
        outline: `
          text-text-secondary bg-bg-input border-0.5 border-border-button
          hover:text-text-primary hover:bg-border-button hover:border-border-default
          focus-visible:text-text-primary focus-visible:bg-border-button focus-visible:border-border-button
        `, // light: bg=transparent, dark: bg-input

        dashed: `
          text-text-secondary border-border-button border-dashed
          hover:text-text-primary hover:bg-border-button hover:border-border-default
          focus-visible:text-text-primary focus-visible:bg-border-button focus-visible:border-border-button
        `,

        icon: 'bg-transparent text-foreground hover:bg-transparent/80',

        transparent: `
          text-text-secondary bg-transparent border-0.5 border-border-button
          hover:text-text-primary hover:bg-border-button
          focus-visible:text-text-primary focus-visible:bg-border-button focus-visible:border-border-button
        `,

        danger: `
          bg-transparent border border-state-error text-state-error
          hover:bg-state-error/10 focus-visible:bg-state-error/10
        `,

        // Ghost variant series
        // Button has transparent background, without borders
        ghost: `
          text-text-secondary
          hover:bg-border-button focus-visible:bg-border-button
          hover:text-text-primary focus-visible:text-text-primary
        `,

        delete: `
          text-text-secondary
          hover:bg-state-error-5 hover:text-state-error
          focus-visible:text-state-error focus-visible:bg-state-error-5
        `,

        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        auto: 'h-full px-1',

        xl: 'h-12 rounded-xl px-5',
        lg: 'h-10 rounded-lg px-4',
        default: 'h-8 rounded px-3',
        sm: 'h-7 rounded-sm px-2',
        xs: 'h-6 rounded-xs px-1',

        'icon-xl': 'size-12 rounded-xl',
        'icon-lg': 'size-10 rounded-lg',
        icon: 'size-8 rounded',
        'icon-sm': 'size-7 rounded-sm',
        'icon-xs': 'size-6 rounded-xs',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

export type ButtonProps<IsAnchor extends boolean = false> = {
  asChild?: boolean;
  asLink?: boolean;
  loading?: boolean;
  block?: boolean;
  disabled?: boolean;
  dot?: boolean;
} & VariantProps<typeof buttonVariants> &
  (IsAnchor extends true
    ? LinkProps
    : React.ButtonHTMLAttributes<HTMLButtonElement>);

const Button = React.forwardRef(
  <IsAnchor extends boolean = false>(
    {
      children,
      className,
      variant,
      size,
      dot = false,
      asChild = false,
      asLink = false,
      loading = false,
      disabled = false,
      block = false,
      ...props
    }: ButtonProps<IsAnchor>,
    ref: React.ForwardedRef<
      IsAnchor extends true ? HTMLAnchorElement : HTMLButtonElement
    >,
  ) => {
    const Comp = asChild ? Slot : asLink ? Link : 'button';

    return (
      <Comp
        className={cn(
          buttonVariants({ variant, size, className }),
          { 'block w-full': block },
          { relative: dot },
        )}
        // @ts-ignore
        ref={ref as React.RefObject<HTMLButtonElement | HTMLAnchorElement>}
        disabled={loading || disabled}
        {...props}
      >
        <>
          {dot && (
            <span className="absolute size-[6px] rounded-full -right-[3px] -top-[3px] bg-state-error animate" />
          )}
          {loading && <LucideLoader2 className="animate-spin" />}
          {children}
        </>
      </Comp>
    );
  },
);

Button.displayName = 'Button';

export const ButtonLoading = Button;

ButtonLoading.displayName = 'ButtonLoading';

export { Button, buttonVariants };

export const BlockButton = React.forwardRef<HTMLButtonElement, ButtonProps>(
  function BlockButton({ children, className, ...props }, ref) {
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
