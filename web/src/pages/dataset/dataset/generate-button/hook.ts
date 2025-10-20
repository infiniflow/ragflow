import message from '@/components/ui/message';
import agentService from '@/services/agent-service';
import kbService, { deletePipelineTask } from '@/services/knowledge-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { t } from 'i18next';
import { useEffect, useState } from 'react';
import { useParams } from 'umi';
import { ProcessingType } from '../../dataset-overview/dataset-common';
import { GenerateType, GenerateTypeMap } from './generate';
export const generateStatus = {
  running: 'running',
  completed: 'completed',
  start: 'start',
  failed: 'failed',
};

enum DatasetKey {
  generate = 'generate',
  pauseGenerate = 'pauseGenerate',
}

export interface ITraceInfo {
  begin_at: string;
  chunk_ids: string;
  create_date: string;
  create_time: number;
  digest: string;
  doc_id: string;
  from_page: number;
  id: string;
  priority: number;
  process_duration: number;
  progress: number;
  progress_msg: string;
  retry_count: number;
  task_type: string;
  to_page: number;
  update_date: string;
  update_time: number;
}

export const useTraceGenerate = ({ open }: { open: boolean }) => {
  const { id } = useParams();
  const [isLoopGraphRun, setLoopGraphRun] = useState(false);
  const [isLoopRaptorRun, setLoopRaptorRun] = useState(false);
  const { data: graphRunData, isFetching: graphRunloading } =
    useQuery<ITraceInfo>({
      queryKey: [GenerateType.KnowledgeGraph, id, open],
      // initialData: {},
      gcTime: 0,
      refetchInterval: isLoopGraphRun ? 5000 : false,
      retry: 3,
      retryDelay: 1000,
      enabled: open,
      queryFn: async () => {
        const { data } = await kbService.traceGraphRag({
          kb_id: id,
        });
        return data?.data || {};
      },
    });

  const { data: raptorRunData, isFetching: raptorRunloading } =
    useQuery<ITraceInfo>({
      queryKey: [GenerateType.Raptor, id, open],
      // initialData: {},
      gcTime: 0,
      refetchInterval: isLoopRaptorRun ? 5000 : false,
      retry: 3,
      retryDelay: 1000,
      enabled: open,
      queryFn: async () => {
        const { data } = await kbService.traceRaptor({
          kb_id: id,
        });
        return data?.data || {};
      },
    });

  useEffect(() => {
    setLoopGraphRun(
      !!(
        (graphRunData?.progress || graphRunData?.progress === 0) &&
        graphRunData?.progress < 1 &&
        graphRunData?.progress >= 0
      ),
    );
  }, [graphRunData?.progress]);

  useEffect(() => {
    setLoopRaptorRun(
      !!(
        (raptorRunData?.progress || raptorRunData?.progress === 0) &&
        raptorRunData?.progress < 1 &&
        raptorRunData?.progress >= 0
      ),
    );
  }, [raptorRunData?.progress]);
  return {
    graphRunData,
    graphRunloading,
    raptorRunData,
    raptorRunloading,
  };
};

export const useUnBindTask = () => {
  const { id } = useParams();
  const { mutateAsync: handleUnbindTask } = useMutation({
    mutationKey: [DatasetKey.pauseGenerate],
    mutationFn: async ({ type }: { type: ProcessingType }) => {
      const { data } = await deletePipelineTask({ kb_id: id as string, type });
      if (data.code === 0) {
        message.success(t('message.operated'));
        // queryClient.invalidateQueries({
        //   queryKey: [type],
        // });
      }
      return data;
    },
  });
  return { handleUnbindTask };
};
export const useDatasetGenerate = () => {
  const queryClient = useQueryClient();
  const { id } = useParams();
  const { handleUnbindTask } = useUnBindTask();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DatasetKey.generate],
    mutationFn: async ({ type }: { type: GenerateType }) => {
      const func =
        type === GenerateType.KnowledgeGraph
          ? kbService.runGraphRag
          : kbService.runRaptor;
      const { data } = await func({
        kb_id: id,
      });
      if (data.code === 0) {
        message.success(t('message.operated'));
        queryClient.invalidateQueries({
          queryKey: [type],
        });
      }
      return data;
    },
  });
  // const pauseGenerate = useCallback(() => {
  //   // TODO: pause generate
  //   console.log('pause generate');
  // }, []);
  const { mutateAsync: pauseGenerate } = useMutation({
    mutationKey: [DatasetKey.pauseGenerate],
    mutationFn: async ({
      task_id,
      type,
    }: {
      task_id: string;
      type: GenerateType;
    }) => {
      const { data } = await agentService.cancelDataflow(task_id);

      const unbindData = await handleUnbindTask({
        type: GenerateTypeMap[type as GenerateType],
      });
      if (data.code === 0 && unbindData.code === 0) {
        // message.success(t('message.operated'));
        queryClient.invalidateQueries({
          queryKey: [type],
        });
      }
      return data;
    },
  });
  return { runGenerate: mutateAsync, pauseGenerate, data, loading };
};
