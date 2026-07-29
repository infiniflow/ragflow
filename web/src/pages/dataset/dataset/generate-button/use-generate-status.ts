import { useMemo } from 'react';

import { GenerateStatus } from './constants';
import { ITraceInfo } from './hook';

export function useGenerateStatus(data?: ITraceInfo) {
  const status = useMemo(() => {
    if (!data) {
      return GenerateStatus.start;
    }
    if (data.progress >= 1) {
      return GenerateStatus.completed;
    } else if (!data.progress && data.progress !== 0) {
      return GenerateStatus.start;
    } else if (data.progress < 0) {
      return GenerateStatus.failed;
    } else if (data.progress < 1) {
      return GenerateStatus.running;
    }
    return GenerateStatus.start;
  }, [data]);

  const percent = useMemo(() => {
    if (status === GenerateStatus.failed) {
      return 100;
    } else if (status === GenerateStatus.running) {
      return data!.progress * 100;
    }
    return 0;
  }, [status, data]);

  return { status, percent };
}
