/**
 * Enterprise slots inside the app header. The open-source build renders
 * nothing here; the enterprise build replaces this file with the real
 * entries (tenant switcher, upgrade badge, support link, current-plan badge).
 */

export function EnterpriseHeaderTenantSwitcher(_props: {
  isCompact?: boolean;
}) {
  void _props;
  return null;
}

export function EnterpriseHeaderUpgradeBadge() {
  return null;
}

export function EnterpriseHeaderTicketsLink() {
  return null;
}

export function EnterpriseHeaderPlanBadge() {
  return null;
}
