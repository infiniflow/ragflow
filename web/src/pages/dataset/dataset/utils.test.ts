import { RunningStatus } from './constant';
import { isParserRunning } from './utils';

describe('isParserRunning', () => {
  it('treats scheduled documents as active parser work', () => {
    expect(isParserRunning(RunningStatus.SCHEDULE)).toBe(true);
  });

  it('does not treat terminal documents as active parser work', () => {
    expect(isParserRunning(RunningStatus.DONE)).toBe(false);
    expect(isParserRunning(RunningStatus.FAIL)).toBe(false);
  });
});
