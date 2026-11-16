import { Card } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import type { ComponentPropsWithoutRef } from 'react';

export function ListPageCard({
  className,
  ...props
}: ComponentPropsWithoutRef<typeof Card>) {
  return (
    <div className="size-full min-h-0 min-w-0 py-2.5 pr-2.5">
      <Card
        className={cn(
          'flex size-full min-h-0 min-w-0 flex-col overflow-hidden rounded-2xl border border-border-default bg-bg-base shadow-none',
          className,
        )}
        {...props}
      />
    </div>
  );
}
