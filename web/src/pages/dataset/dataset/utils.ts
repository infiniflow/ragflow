import { RunningStatus } from './constant';

export const isParserRunning = (text: RunningStatus) => {
  const isRunning = text === RunningStatus.RUNNING;
  return isRunning;
};
