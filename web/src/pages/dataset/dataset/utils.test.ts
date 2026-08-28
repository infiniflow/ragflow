import { RunningStatus } from './constant';
import { isParserRunning } from './utils';

describe('isParserRunning', () => {
  it.each([RunningStatus.RUNNING, RunningStatus.SCHEDULE])(
    'treats %s as active parsing',
    (status) => {
      expect(isParserRunning(status)).toBe(true);
    },
  );

  it.each([
    RunningStatus.UNSTART,
    RunningStatus.CANCEL,
    RunningStatus.DONE,
    RunningStatus.FAIL,
  ])('does not treat %s as active parsing', (status) => {
    expect(isParserRunning(status)).toBe(false);
  });
});
