import { Card } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';

type PageFrameProps = PropsWithChildren<{
  className?: string;
}>;

export function PageFrame({ children, className }: PageFrameProps) {
  return (
    <div className="page-frame h-full pr-2.5 pb-2.5">
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
