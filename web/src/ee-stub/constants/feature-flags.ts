/**
 * Enterprise seam for capability flags that shared pages read at build time.
 *
 * Open source ships the value used by the public build; the enterprise tree
 * replaces this file and turns the capability off.
 */

/**
 * Whether resources (agents, datasets, memories) expose a `permission` field
 * for choosing between private and team ownership.
 */
export const showPermissionField = true;
