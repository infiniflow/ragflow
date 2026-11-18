import type { IntegrationChild } from '@/layouts/components/app-sidebar/navigation';

export function useVisibleIntegrationChildren(
  children: IntegrationChild[],
): IntegrationChild[] {
  return children;
}
