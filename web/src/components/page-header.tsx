import { PropsWithChildren } from 'react';

export function PageHeader({ children }: PropsWithChildren) {
  return (
    <header className="flex justify-between items-center border-b bg-background-header-bar p-5">
      {children}
    </header>
  );
}
