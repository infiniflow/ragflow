import {
  normalizeRunningStatus,
  RunningStatus,
  RunningStatusOld,
} from '@/constants/knowledge';
import { isParserRunning } from './utils';

describe('document running status compatibility', () => {
  it.each([
    [RunningStatus.RUNNING, RunningStatus.RUNNING],
    [RunningStatusOld.RUNNING, RunningStatus.RUNNING],
    [1, RunningStatus.RUNNING],
    [0, RunningStatus.UNSTART],
    [5, RunningStatus.SCHEDULE],
  ])('normalizes %p to %p', (value, expected) => {
    expect(normalizeRunningStatus(value)).toBe(expected);
  });

  it('only treats every representation of running as running', () => {
    expect(isParserRunning(1)).toBe(true);
    expect(isParserRunning(RunningStatusOld.RUNNING)).toBe(true);
    expect(isParserRunning(RunningStatus.DONE)).toBe(false);
    expect(isParserRunning('unexpected' as never)).toBe(false);
  });
});
