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

/**
 * Whether the ingestion (dataflow) canvas exposes the Run (debug) entry.
 * The agent canvas is unaffected and always keeps its Run button.
 */
export const showPipelineRunButton = false;
