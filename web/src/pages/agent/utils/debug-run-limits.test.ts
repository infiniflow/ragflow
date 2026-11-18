import { debugRunLimitsTooltipKey } from './debug-run-limits';

describe('debugRunLimitsTooltipKey', () => {
  it('returns the key for a dataflow (ingestion pipeline) canvas', () => {
    expect(debugRunLimitsTooltipKey(true)).toBe('flow.debugRunLimits');
  });

  it('returns null for an agent canvas (no ingestion debug preview)', () => {
    expect(debugRunLimitsTooltipKey(false)).toBeNull();
  });
});
