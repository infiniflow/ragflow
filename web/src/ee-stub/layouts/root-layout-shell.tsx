import React, { PropsWithChildren, useMemo } from 'react';

export type RootLayoutChrome = {
  showHeader: boolean;
  containerRef?: React.Ref<HTMLDivElement>;
};

/**
 * Enterprise seams for the application shell. Open source has no notification
 * banner, no billing wrapper and always shows the header, so every seam leaves
 * the upstream layout untouched.
 */
export function RootLayoutBanner() {
  return null;
}

export function RootLayoutContent({ children }: PropsWithChildren) {
  return <>{children}</>;
}

export function useRootLayoutChrome(): RootLayoutChrome {
  return useMemo(() => ({ showHeader: true }), []);
}
