/**
 * debugRunLimitsTooltipKey returns the i18n key for the canvas "Run" button
 * tooltip describing the debug (dry-run) preview limits, or null when the
 * tooltip should not be shown.
 *
 * The tooltip applies ONLY to a dataflow (ingestion pipeline) canvas. An
 * agent canvas runs the agent/chat, not an ingestion debug preview, so it
 * must never show this tooltip.
 */
export const debugRunLimitsTooltipKey = (isPipeline: boolean): string | null =>
  isPipeline ? 'flow.debugRunLimits' : null;
