import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';

type DatasetConfigurationContainerProps = {
  className?: string;
  show?: boolean;
} & PropsWithChildren;

export function DatasetConfigurationContainer({
  children,
  className,
  show = true,
}: DatasetConfigurationContainerProps) {
  return show ? (
    <div
      className={cn(
        'border p-2 rounded-lg bg-slate-50 dark:bg-gray-600',
        className,
      )}
    >
      {children}
    </div>
  ) : (
    children
  );
}
