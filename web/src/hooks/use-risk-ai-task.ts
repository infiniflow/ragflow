import { IRiskAITask } from '@/interfaces/knowledge/risk-ai-task';
import {
  createRiskAITask,
  getRiskAITaskStatus,
} from '@/services/knowledge-service';
import { useCallback, useEffect, useRef, useState } from 'react';

export const useRiskAiTask = () => {
  const [task, setTask] = useState<IRiskAITask | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pollTimer = useRef<NodeJS.Timeout | null>(null);

  const clearTimer = useCallback(() => {
    if (pollTimer.current) {
      clearTimeout(pollTimer.current);
      pollTimer.current = null;
    }
  }, []);

  const parseResponse = async (res: any) => {
    if (res && typeof res === 'object') {
      if ('data' in res && res.data) {
        return res.data;
      }
      if (typeof res.json === 'function') {
        return await res.json();
      }
    }
    return res;
  };

  const pollStatus = useCallback(
    (taskId: string) => {
      const fetchStatus = async () => {
        try {
          const res = await getRiskAITaskStatus(taskId);
          const payload = await parseResponse(res);
          if (Number(payload?.code) === 0) {
            setError(null);
            const taskData = payload.data as IRiskAITask;
            setTask(taskData);
            if (['pending', 'running'].includes(taskData.status)) {
              pollTimer.current = setTimeout(fetchStatus, 3000);
            } else {
              clearTimer();
            }
          } else {
            setError(payload?.message || 'Task status error');
            clearTimer();
          }
        } catch (err) {
          setError((err as Error).message);
          clearTimer();
        }
      };
      fetchStatus();
    },
    [clearTimer, parseResponse],
  );

  const startTask = useCallback(
    async (kbId: string, rows: any[], options?: Record<string, any>) => {
      setLoading(true);
      setError(null);
      clearTimer();
      try {
        const res = await createRiskAITask(kbId, rows, options);
        const payload = await parseResponse(res);
        if (Number(payload?.code) === 0) {
          setError(null);
          const taskId = payload.data.task_id as string;
          setTask({
            id: taskId,
            kb_id: kbId,
            status: 'pending',
            created_by: '',
          });
          pollStatus(taskId);
        } else {
          setError(payload?.message || 'Failed to create task');
        }
      } catch (err) {
        setError((err as Error).message);
      }
      setLoading(false);
    },
    [clearTimer, pollStatus, parseResponse],
  );

  useEffect(() => () => clearTimer(), [clearTimer]);

  return {
    task,
    loading,
    error,
    startTask,
    pollStatus,
  };
};
