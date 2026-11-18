import type { ReactNode } from 'react';

type ModelSharingBadgeProps = {
  tenantName?: string;
  tenantId?: string;
  currentTenantId?: string;
  userId?: string;
  nickname?: string;
  currentUserId?: string;
};

/** Keeps the community model selector's tenant label unchanged. */
export function ModelSharingBadge({
  tenantName,
  tenantId,
  currentTenantId,
}: ModelSharingBadgeProps): ReactNode {
  if (!tenantName || tenantId === currentTenantId) return null;

  return (
    <span className="text-text-secondary bg-bg-card rounded-sm px-1 truncate flex-shrink-0">
      {tenantName}
    </span>
  );
}
