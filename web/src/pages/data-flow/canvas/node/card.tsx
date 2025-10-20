import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';

type LabelCardProps = {
  className?: string;
} & PropsWithChildren;

export function LabelCard({ children, className }: LabelCardProps) {
  return (
    <div className={cn('bg-bg-card rounded-sm p-1', className)}>{children}</div>
  );
}
