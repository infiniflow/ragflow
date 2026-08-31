import {
  normalizeRunningStatus,
  RunningStatus,
  RunningStatusValue,
} from '@/constants/knowledge';

export const isParserRunning = (status: RunningStatusValue) => {
  return normalizeRunningStatus(status) === RunningStatus.RUNNING;
};
