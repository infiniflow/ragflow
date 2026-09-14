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
  // Legacy scheduled state; rendered like the Go queued state.
  [RunningStatus.SCHEDULE]: {
    label: 'SCHEDULE',
    color: 'rgba(var(--state-warning))',
  },
  // Go ingestion only: task enqueued but not started (CREATED/SCHEDULED).
  [RunningStatus.QUEUED]: {
    label: 'QUEUED',
    color: 'rgba(var(--state-warning))',
  },
};

export * from '@/constants/knowledge';
