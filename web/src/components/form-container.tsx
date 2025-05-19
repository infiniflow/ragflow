import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';

export type FormContainerProps = {
  className?: string;
  show?: boolean;
} & PropsWithChildren;

export function FormContainer({
  children,
  show = true,
  className,
}: FormContainerProps) {
  return show ? (
    <section className={cn('border rounded-lg p-5 space-y-5', className)}>
      {children}
    </section>
  ) : (
    children
  );
}
