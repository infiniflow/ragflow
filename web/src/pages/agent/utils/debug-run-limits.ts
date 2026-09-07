/**
 * debugRunLimitsTooltipKey returns the i18n key for the canvas "Run" button
 * tooltip describing the debug (dry-run) preview limits, or null when the
 * tooltip should not be shown.
 *
 * The tooltip applies ONLY to a dataflow (ingestion pipeline) canvas on the
 * golang backend. An agent canvas runs the agent/chat, not an ingestion
 * debug preview, so it must never show this tooltip. The python backend's
 * debug semantics also differ and are out of scope.
 *
 * It is a pure function of (backend language, is-pipeline) so it can be
 * unit-tested without mocking; the caller reads the backend through
 * `useIsGoBackend()`.
 *
 * The tooltip copy itself never names the backend language; only its
 * visibility is gated on these conditions.
 */
export const debugRunLimitsTooltipKey = (
  isGo: boolean,
  isPipeline: boolean,
): string | null => (isGo && isPipeline ? 'flow.debugRunLimits' : null);
