import { PropsWithChildren } from 'react';

export function Subhead({ children }: PropsWithChildren) {
  return (
    <div className="text-xl font-bold mb-4 text-colors-text-neutral-strong">
      {children}
    </div>
  );
}
