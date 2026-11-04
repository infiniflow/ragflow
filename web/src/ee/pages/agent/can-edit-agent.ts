/**
 * Enterprise seam for canvas edit permissions. Open source has no collaborator
 * permissions, so every user may edit and autosave the agent.
 */
export function canEditAgent(_operatorPermission: number) {
  void _operatorPermission;
  return true;
}
