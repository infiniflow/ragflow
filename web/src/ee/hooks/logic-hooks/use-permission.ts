/**
 * Enterprise permission seam. The enterprise build overlays this file with
 * role-based gating; open source has no edit restrictions.
 */

export function useDatasetEditButtonDisabled() {
  return false;
}

export function useDatasetManageButtonDisabled() {
  return false;
}
