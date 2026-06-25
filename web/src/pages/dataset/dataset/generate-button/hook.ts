import message from '@/components/ui/message';
import agentService from '@/services/agent-service';
import {
  deletePipelineTask,
  runIndex,
  traceIndex,
} from '@/services/knowledge-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { t } from 'i18next';
import { useParams } from 'react-router';
import { ProcessingType } from '../../dataset-overview/dataset-common';
import { GenerateType, GenerateTypeMap } from './constants';

enum DatasetKey {
  generate = 'generate',
  pauseGenerate = 'pauseGenerate',
}

const PollIntervalMs = 5000;

type TraceType = 'graph' | 'raptor';

const DatasetGenerateKeys = {
  trace: (type: GenerateType, id?: string, open?: boolean) =>
    [type, id, open] as const,
  traceById: (type: GenerateType, id?: string) => [type, id] as const,
};

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

const useTraceQuery = (
  type: GenerateType,
  traceType: TraceType,
  open: boolean,
  id?: string,
) => {
  return useQuery<ITraceInfo>({
    queryKey: DatasetGenerateKeys.trace(type, id, open),
    gcTime: 0,
    refetchInterval: (query) => {
      const progress = query.state.data?.progress;
      return progress != null && progress >= 0 && progress < 1
        ? PollIntervalMs
        : false;
    },
    retry: 3,
    retryDelay: 1000,
    enabled: open && !!id,
    queryFn: async () => {
      const { data } = await traceIndex(id!, traceType);
      return data?.data ?? {};
    },
  });
};

export const useTraceGenerate = ({ open }: { open: boolean }) => {
  const { id } = useParams();
  const { data: graphRunData, isFetching: graphRunLoading } = useTraceQuery(
    GenerateType.KnowledgeGraph,
    'graph',
    open,
    id,
  );
  const { data: raptorRunData, isFetching: raptorRunLoading } = useTraceQuery(
    GenerateType.Raptor,
    'raptor',
    open,
    id,
  );

  return {
    graphRunData,
    graphRunLoading,
    raptorRunData,
    raptorRunLoading,
  };
};

export const useUnBindTask = () => {
  const { id } = useParams();
  const { mutateAsync: handleUnbindTask } = useMutation({
    mutationKey: [DatasetKey.pauseGenerate],
    mutationFn: async ({
      type,
      wipe,
    }: {
      type: ProcessingType;
      wipe?: boolean;
    }) => {
      const { data } = await deletePipelineTask({
        kb_id: id as string,
        type,
        wipe,
      });
      if (data.code === 0) {
        message.success(t('message.operated'));
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
      const indexType =
        type === GenerateType.KnowledgeGraph ? 'graph' : 'raptor';
      const { data } = await runIndex(id!, indexType);
      if (data.code === 0) {
        message.success(t('message.operated'));
        queryClient.invalidateQueries({
          queryKey: DatasetGenerateKeys.traceById(type, id),
        });
      }
      return data;
    },
  });

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

      // For GraphRAG, pause must preserve partial progress (subgraphs,
      // entities, relations, community reports) so the next run_graphrag
      // call can resume instead of redoing hours of LLM extraction. Raptor
      // keeps the prior wipe-on-pause behaviour for now.
      const unbindData = await handleUnbindTask({
        type: GenerateTypeMap[type as GenerateType],
        wipe: type === GenerateType.KnowledgeGraph ? false : undefined,
      });
      if (data.code === 0 && unbindData.code === 0) {
        queryClient.invalidateQueries({
          queryKey: DatasetGenerateKeys.traceById(type, id),
        });
      }
      return data;
    },
  });
  return { runGenerate: mutateAsync, pauseGenerate, data, loading };
};
