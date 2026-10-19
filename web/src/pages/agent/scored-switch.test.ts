import {
  showsScoredSwitchEscalateHandle,
} from './scored-switch';

describe('showsScoredSwitchEscalateHandle', () => {
  it('shows the escalate handle by default', () => {
    expect(showsScoredSwitchEscalateHandle(undefined)).toBe(true);
    expect(showsScoredSwitchEscalateHandle(true)).toBe(true);
  });

  it('hides the escalate handle when escalation is off', () => {
    expect(showsScoredSwitchEscalateHandle(false)).toBe(false);
  });
});
