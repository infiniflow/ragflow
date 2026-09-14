import { RunningStatus } from '@/constants/knowledge';

export const RunningStatusMap = {
  [RunningStatus.UNSTART]: {
    label: 'UNSTART',
    color: 'rgba(var(--accent-primary))',
  },
  [RunningStatus.RUNNING]: {
    label: 'Parsing',
    color: 'var(--team-member)',
  },
  [RunningStatus.CANCEL]: {
    label: 'CANCEL',
    color: 'rgba(var(--state-warning))',
  },
  [RunningStatus.DONE]: {
    label: 'SUCCESS',
    color: 'rgba(var(--state-success))',
  },
  [RunningStatus.FAIL]: { label: 'FAIL', color: 'rgba(var(--state-error))' },
  [RunningStatus.SCHEDULE]: {
    label: 'SCHEDULE',
    color: 'rgba(var(--team-member))',
  },
};

export * from '@/constants/knowledge';
