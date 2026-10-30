export const ScoredSwitchEscalateHandleId = 'escalate';

export const ScoredSwitchDefaultMarginThreshold = 0.15;

export const ScoredSwitchDefaultMinTopScore = 0;

export function showsScoredSwitchEscalateHandle(
  value: unknown,
  fieldMode?: unknown,
): boolean {
  if (fieldMode === true) {
    return false;
  }
  return value !== false;
}
