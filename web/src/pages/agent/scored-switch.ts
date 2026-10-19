export const ScoredSwitchEscalateHandleId = 'escalate';

export const ScoredSwitchDefaultMarginThreshold = 0.15;

export const ScoredSwitchDefaultMinTopScore = 0;

export function showsScoredSwitchEscalateHandle(value: unknown): boolean {
  return value !== false;
}
