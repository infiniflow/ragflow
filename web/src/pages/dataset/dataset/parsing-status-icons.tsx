import { IconFontFill } from '@/components/icon-font';
import { CircleX } from 'lucide-react';
import { RunningStatus } from './constant';

// Shared by the Go/Python parse-status cells so the run -> operation-icon and
// run -> data-state mappings cannot drift between backends.
export const StatusOperationIcon = {
  [RunningStatus.UNSTART]: (
    <IconFontFill name="play" className="text-accent-primary size-[1em]" />
  ),
  [RunningStatus.RUNNING]: (
    <CircleX color="rgba(var(--state-error))" className="size-[1em]" />
  ),
  [RunningStatus.CANCEL]: (
    <IconFontFill name="reparse" className="text-accent-primary" />
  ),
  [RunningStatus.DONE]: (
    <IconFontFill name="reparse" className="text-accent-primary" />
  ),
  [RunningStatus.FAIL]: (
    <IconFontFill name="reparse" className="text-accent-primary" />
  ),
  [RunningStatus.SCHEDULE]: (
    <IconFontFill name="reparse" className="text-accent-primary" />
  ),
};

export const ParseStatusStateMap = {
  [RunningStatus.UNSTART]: 'unstart',
  [RunningStatus.RUNNING]: 'running',
  [RunningStatus.CANCEL]: 'cancel',
  [RunningStatus.DONE]: 'success',
  [RunningStatus.FAIL]: 'fail',
  [RunningStatus.SCHEDULE]: 'running',
} as const;
