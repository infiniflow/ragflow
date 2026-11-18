import { Card } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';

type DatasetPageFrameProps = PropsWithChildren<{
  className?: string;
}>;

export function DatasetPageFrame({
  children,
  className,
}: DatasetPageFrameProps) {
  return (
    <div className="h-full pr-5 pb-2.5">
      <Card
        as="article"
        className={cn(
          'h-full bg-transparent shadow-none flex flex-col min-h-0 overflow-hidden',
          className,
        )}
      >
        {children}
      </Card>
    </div>
  );
}
