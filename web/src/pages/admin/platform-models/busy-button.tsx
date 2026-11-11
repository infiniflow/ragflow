import { forwardRef, MouseEvent, useCallback } from 'react';

import { LucideLoader2 } from 'lucide-react';

import { Button, ButtonProps } from '@/components/ui/button';

/**
 * A button that stays focusable while its action runs. A disabled button
 * would drop keyboard focus to the page; this one is marked busy and ignores
 * clicks (and form submits) until the action ends.
 */
export const BusyButton = forwardRef<
  HTMLButtonElement,
  ButtonProps & { busy?: boolean }
>(function BusyButton({ busy, onClick, children, ...props }, ref) {
  const handleClick = useCallback(
    (event: MouseEvent<HTMLButtonElement>) => {
      if (busy) {
        event.preventDefault();
        return;
      }
      onClick?.(event);
    },
    [busy, onClick],
  );
  return (
    <Button
      ref={ref}
      {...props}
      aria-disabled={busy || undefined}
      aria-busy={busy || undefined}
      onClick={handleClick}
    >
      {busy && <LucideLoader2 className="animate-spin" />}
      {children}
    </Button>
  );
});
