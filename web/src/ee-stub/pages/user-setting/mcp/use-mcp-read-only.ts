/**
 * Enterprise seam for the MCP server edit permission. Open source has no
 * collaborator permissions, so every visible server stays editable.
 */
export function useMcpReadOnly(_operatorPermission?: number) {
  void _operatorPermission;
  return false;
}
