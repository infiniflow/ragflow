/**
 * Enterprise seam for resource-level permission checks.
 *
 * The enterprise build replaces this file with collaborator-aware checks based
 * on `operator_permission`. Open source has no collaborator permissions: every
 * resource belongs to its tenant and every user who can see it can edit and
 * delete it.
 */

export function hasPreviewPermission(operatorPermission: number) {
  void operatorPermission;
  return false;
}

export function hasEditPermission(operatorPermission: number) {
  void operatorPermission;
  return true;
}

export function hasManagementPermission(operatorPermission: number) {
  void operatorPermission;
  return true;
}

export function hasOwnerPermission(operatorPermission: number) {
  void operatorPermission;
  return true;
}

export function showEditButton(operatorPermission: number) {
  void operatorPermission;
  return true;
}

export function disableEditButton(operatorPermission: number) {
  void operatorPermission;
  return false;
}

export function hasManagePermissionPermission(operatorPermission: number) {
  void operatorPermission;
  return true;
}
