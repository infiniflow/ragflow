import { FormContainerProps } from '@/components/form-container';
import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';

export function ConfigurationFormContainer({
  children,
  className,
}: FormContainerProps) {
  return <section className={cn('space-y-4', className)}>{children}</section>;
}

export function MainContainer({ children }: PropsWithChildren) {
  return <section className="space-y-5">{children}</section>;
}
