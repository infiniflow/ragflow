import { FormContainer, FormContainerProps } from '@/components/form-container';
import { cn } from '@/lib/utils';
import { PropsWithChildren } from 'react';

export function ConfigurationFormContainer({
  children,
  className,
}: FormContainerProps) {
  return (
    <FormContainer className={cn('p-10', className)}>{children}</FormContainer>
  );
}

export function MainContainer({ children }: PropsWithChildren) {
  return <section className="space-y-5">{children}</section>;
}
