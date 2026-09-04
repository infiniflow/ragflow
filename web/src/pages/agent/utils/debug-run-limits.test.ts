import { debugRunLimitsTooltipKey } from './debug-run-limits';

describe('debugRunLimitsTooltipKey', () => {
  it('returns the key only for a dataflow (ingestion pipeline) canvas on golang', () => {
    expect(debugRunLimitsTooltipKey(true, true)).toBe('flow.debugRunLimits');
  });

  it('returns null for an agent canvas even on golang (no ingestion debug preview)', () => {
    expect(debugRunLimitsTooltipKey(true, false)).toBeNull();
  });

  it('returns null on the python backend even for a dataflow canvas', () => {
    expect(debugRunLimitsTooltipKey(false, true)).toBeNull();
  });

  it('returns null when neither condition holds', () => {
    expect(debugRunLimitsTooltipKey(false, false)).toBeNull();
  });
});
