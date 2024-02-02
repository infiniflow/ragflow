import { RunningStatus } from '@/constants/knowledge';

export const RunningStatusMap = {
  [RunningStatus.UNSTART]: {
    label: 'UNSTART',
    color: 'cyan',
  },
  [RunningStatus.RUNNING]: {
    label: 'Parsing',
    color: 'blue',
  },
  [RunningStatus.CANCEL]: { label: 'CANCEL', color: 'orange' },
  [RunningStatus.DONE]: { label: 'SUCCESS', color: 'geekblue' },
  [RunningStatus.FAIL]: { label: 'FAIL', color: 'red' },
};

export * from '@/constants/knowledge';
