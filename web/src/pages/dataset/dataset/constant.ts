import { RunningStatus } from '@/constants/knowledge';

export const RunningStatusMap = {
  [RunningStatus.UNSTART]: {
    label: 'UNSTART',
    color: 'var(--accent-primary)',
  },
  [RunningStatus.RUNNING]: {
    label: 'Parsing',
    color: 'var(--team-member)',
  },
  [RunningStatus.CANCEL]: { label: 'CANCEL', color: 'var(--state-warning)' },
  [RunningStatus.DONE]: { label: 'SUCCESS', color: 'var(--state-success)' },
  [RunningStatus.FAIL]: { label: 'FAIL', color: 'var(--state-error' },
};

export * from '@/constants/knowledge';
