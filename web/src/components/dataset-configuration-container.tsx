import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';

type DatasetConfigurationContainerProps = {
  className?: string;
} & PropsWithChildren;

export function DatasetConfigurationContainer({
  children,
  className,
}: DatasetConfigurationContainerProps) {
  return (
    <div
      className={cn(
        'border p-2 rounded-lg bg-slate-50 dark:bg-gray-600',
        className,
      )}
    >
      {children}
    </div>
  );
}
