'use client';

import * as CheckboxPrimitive from '@radix-ui/react-checkbox';
import { LucideCheck } from 'lucide-react';
import * as React from 'react';

import { cn } from '@/lib/utils';

const Checkbox = React.forwardRef<
  React.ElementRef<typeof CheckboxPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>
>(({ className, ...props }, ref) => (
  <CheckboxPrimitive.Root
    ref={ref}
    className={cn(
      'peer size-3.5 shrink-0 rounded-2xs border border-text-disabled outline-0 transition-colors bg-transparent',
      'hover:border-border-default hover:bg-border-button',
      'focus-visible:border-border-default focus-visible:bg-border-default',
      'disabled:cursor-not-allowed disabled:opacity-50',
      'data-[state=checked]:text-text-primary data-[state=checked]:border-text-primary',
      className,
    )}
    {...props}
  >
    <CheckboxPrimitive.Indicator className="flex items-center justify-center text-current">
      <LucideCheck className="size-2.5 stroke-[3]" />
    </CheckboxPrimitive.Indicator>
  </CheckboxPrimitive.Root>
));
Checkbox.displayName = CheckboxPrimitive.Root.displayName;

export { Checkbox };
