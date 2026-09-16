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

/** Why a file cannot be parsed under the dataset's current Parser operator. */
export enum ParserGapReason {
  // The operator setups declare no entry for the file's type family at all.
  UnsupportedType = 'unsupportedType',
  // The family is declared, but its required model (audio/video) is not set.
  MissingModel = 'missingModel',
}

/**
 * Model capability a Parser operator setup needs for audio/video files.
 * Values follow the parser-configuration vocabulary (the ModelTypeToField
 * keys), not the LLM registry types (LlmModelType).
 */
export enum ParserModelKind {
  Asr = 'asr',
  Vision = 'vision',
}
