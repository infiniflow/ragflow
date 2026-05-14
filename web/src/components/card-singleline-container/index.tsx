import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';
import './index.less';

type CardContainerProps = { className?: string } & PropsWithChildren;

export function CardSineLineContainer({
  children,
  className,
}: CardContainerProps) {
  return (
    <section
      className={cn(
        'grid gap-6 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6 single',
        className,
      )}
    >
      {children}
    </section>
  );
}
