import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';

// Helper functions for progress dismissal storage
const getDismissalKey = (knowledgeBaseId: string, progressType: string) =>
  `progress_dismissed_${knowledgeBaseId}_${progressType}`;

const isProgressDismissed = (
  knowledgeBaseId: string,
  progressType: string,
): boolean => {
  const key = getDismissalKey(knowledgeBaseId, progressType);
  return localStorage.getItem(key) === 'true';
};

const setProgressDismissed = (
  knowledgeBaseId: string,
  progressType: string,
): void => {
  const key = getDismissalKey(knowledgeBaseId, progressType);
  localStorage.setItem(key, 'true');
};

const clearProgressDismissal = (
  knowledgeBaseId: string,
  progressType: string,
): void => {
  const key = getDismissalKey(knowledgeBaseId, progressType);
  localStorage.removeItem(key);
};

interface ProgressData {
  current_status: string;
  [key: string]: any;
}

interface UseProgressPollingParams<T> {
  knowledgeBaseId: string;
  operationName: string;
  progressEndpoint: (
    id: string,
  ) => Promise<{ data: { code: number; data: T } }>;
  mutation: any; // Replace with a more specific type if possible
  initialProgressState: T;
  onSuccessMessage: string;
}

export const useProgressPolling = <T extends ProgressData>({
  knowledgeBaseId,
  operationName,
  progressEndpoint,
  mutation,
  initialProgressState,
  onSuccessMessage,
}: UseProgressPollingParams<T>) => {
  const [progress, setProgress] = useState<T | null>(null);
  const pollingRef = useRef<NodeJS.Timeout | null>(null);
  const queryClient = useQueryClient();
  const { isPending: loading, mutateAsync, data } = mutation;

  const startPolling = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
    }

    pollingRef.current = setInterval(async () => {
      try {
        const { data: progressData } = await progressEndpoint(knowledgeBaseId);
        if (progressData.code === 0 && progressData.data) {
          if (
            progressData.data.current_status === 'completed' &&
            isProgressDismissed(knowledgeBaseId, operationName)
          ) {
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            return;
          }

          setProgress(progressData.data);

          if (progressData.data.current_status === 'completed') {
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            queryClient.invalidateQueries({
              queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
            });
          }
        } else if (progressData.code === 0 && progressData.data === null) {
          if (pollingRef.current) {
            clearInterval(pollingRef.current);
            pollingRef.current = null;
          }
        }
      } catch (error) {
        console.error(`Failed to fetch ${operationName} progress:`, error);
      }
    }, 3000);
  };

  useEffect(() => {
    const checkInitialProgress = async () => {
      try {
        const { data: progressData } = await progressEndpoint(knowledgeBaseId);

        if (progressData.code === 0 && progressData.data) {
          if (
            progressData.data.current_status === 'completed' &&
            isProgressDismissed(knowledgeBaseId, operationName)
          ) {
            return;
          }

          setProgress(progressData.data);

          if (progressData.data.current_status !== 'completed') {
            startPolling();
          }
        }
      } catch (error) {
        console.error(
          `Failed to check initial ${operationName} progress:`,
          error,
        );
      }
    };

    if (knowledgeBaseId) {
      checkInitialProgress();
    }
  }, [knowledgeBaseId]);

  useEffect(() => {
    if (loading) {
      clearProgressDismissal(knowledgeBaseId, operationName);
      setProgress(initialProgressState);
      startPolling();
    }
  }, [loading]);

  useEffect(() => {
    return () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [knowledgeBaseId]);

  return {
    data,
    loading,
    runOperation: mutateAsync,
    progress,
    clearProgress: () => {
      setProgress(null);
      setProgressDismissed(knowledgeBaseId, operationName);
    },
  };
};
