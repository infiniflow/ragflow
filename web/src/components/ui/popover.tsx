'use client';

import * as PopoverPrimitive from '@radix-ui/react-popover';
import * as React from 'react';

import { cn } from '@/lib/utils';

interface PopoverProps extends PopoverPrimitive.PopoverProps {
  disableOutsideClick?: boolean;
}

const Popover = ({
  children,
  open: openState,
  onOpenChange,
  disableOutsideClick = false,
}: PopoverProps) => {
  // Initialize from the prop instead of a hard-coded `true`: the latter
  // renders every popover open for the first frame(s) until the effect syncs,
  // and Radix's close sequence then steals focus to the trigger — scrolling
  // any scrollable ancestor (e.g. the dataset configuration form) to it.
  const [open, setOpen] = React.useState(!!openState);
  React.useEffect(() => {
    setOpen(!!openState);
  }, [openState]);
  const handleOnOpenChange = React.useCallback(
    (e: boolean) => {
      if (disableOutsideClick && !e) {
        return;
      }

      if (onOpenChange) {
        onOpenChange?.(e);
      }
      setOpen(e);
    },
    [onOpenChange, disableOutsideClick],
  );
  return (
    <PopoverPrimitive.Root open={open} onOpenChange={handleOnOpenChange}>
      {children}
    </PopoverPrimitive.Root>
  );
};

const PopoverTrigger = PopoverPrimitive.Trigger;

const PopoverContent = React.forwardRef<
  React.ElementRef<typeof PopoverPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof PopoverPrimitive.Content>
>(({ className, align = 'center', sideOffset = 4, ...props }, ref) => (
  <PopoverPrimitive.Portal>
    <PopoverPrimitive.Content
      ref={ref}
      align={align}
      sideOffset={sideOffset}
      className={cn(
        'z-50 w-72 rounded-md border-0.5 border-border-button bg-bg-base p-4 text-text-primary shadow-lg outline-none',
        'data-[state=open]:animate-in data-[state=closed]:animate-out',
        'data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
        'data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-9',
        'data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
        className,
      )}
      {...props}
    />
  </PopoverPrimitive.Portal>
));
PopoverContent.displayName = PopoverPrimitive.Content.displayName;

export { Popover, PopoverContent, PopoverTrigger };
