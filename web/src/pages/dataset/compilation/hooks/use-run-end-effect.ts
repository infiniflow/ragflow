import { GenerateStatus } from '@/constants/knowledge';
import { useDebounceFn } from 'ahooks';
import { useEffect, useRef } from 'react';

// Fires once when a compilation run leaves the Running state, no matter how it
// ended: finished at 100% (Running -> Completed), paused (the task is unbound,
// so the trace resets and Running -> Start), or failed. Views refresh their
// data through this single path so pausing behaves exactly like completion.
// Debounced to fire 1s after the run settles, so the refetch reads final state.
export function useRunEndEffect(status: GenerateStatus, onRunEnd: () => void) {
  const prevStatusRef = useRef(status);
  const { run: scheduleRunEnd } = useDebounceFn(onRunEnd, { wait: 2000 });

  useEffect(() => {
    const prevStatus = prevStatusRef.current;
    prevStatusRef.current = status;
    if (
      prevStatus === GenerateStatus.Running &&
      status !== GenerateStatus.Running
    ) {
      scheduleRunEnd();
    }
  }, [status, scheduleRunEnd]);
}
