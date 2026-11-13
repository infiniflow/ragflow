import { ReactNode } from 'react';

/**
 * Provider wrapped around portal modals. The enterprise build replaces this
 * file with the React Query provider those modals need; open source renders
 * children directly.
 */
export function EnterpriseModalProvider({ children }: { children: ReactNode }) {
  return <>{children}</>;
}
