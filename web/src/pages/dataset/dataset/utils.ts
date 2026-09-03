import { RunningStatus } from './constant';

export const isParserRunning = (text: RunningStatus) => {
  const isRunning =
    text === RunningStatus.RUNNING || text === RunningStatus.SCHEDULE;
  return isRunning;
};
