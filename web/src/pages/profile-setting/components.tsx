import { PropsWithChildren } from 'react';

export function Title({ children }: PropsWithChildren) {
  return <span className="font-bold text-xl">{children}</span>;
}
