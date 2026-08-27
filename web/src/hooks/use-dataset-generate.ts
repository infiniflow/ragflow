/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import message from '@/components/ui/message';
import {
  GenerateStatus,
  GenerateType,
  GenerateTypeMap,
  ProcessingType,
  TraceType,
} from '@/constants/knowledge';
import agentService from '@/services/agent-service';
import {
  deletePipelineTask,
  getDatasetCompilationStatus,
  runIndex,
  traceIndex,
} from '@/services/knowledge-service';
import { useIsGoBackend } from '@/utils/backend-variant';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

enum DatasetKey {
  generate = 'generate',
  pauseGenerate = 'pauseGenerate',
}

const PollIntervalMs = 5000;

export const DatasetGenerateKeys = {
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
  // Go scheduler compile-status contract. These
  // replace the legacy task percentage for the Go backend: state is the raw
  // dataset-level lifecycle (idle/pending/running/completed), inflight/backlog
  // are the MySQL scheduling-entry counts, and error is the batch diagnostic
  // (NOT a peer state). Only populated by the Go branch of useTraceQuery.
  compilationState?: string;
  currentPhase?: string;
  inflight?: number;
  backlog?: number;
  compilationError?: string;
}

const useTraceQuery = (
  type: GenerateType,
  traceType: TraceType,
  open: boolean,
  id?: string,
) => {
  const isGo = useIsGoBackend();
  return useQuery<ITraceInfo>({
    queryKey: DatasetGenerateKeys.trace(type, id, open),
    gcTime: 0,
    refetchInterval: (query) => {
      const progress = query.state.data?.progress;
      // Go: keep polling while the dataset compile is pending/running
      // (a failed batch is left for retry, so the row stays running/pending and
      // we keep polling until it drains to completed).
      if (isGo) {
        const state = query.state.data?.compilationState;
        return state === 'pending' || state === 'running'
          ? PollIntervalMs
          : false;
      }
      return progress != null && progress >= 0 && progress < 1
        ? PollIntervalMs
        : false;
    },
    retry: 3,
    retryDelay: 1000,
    enabled: open && !!id,
    queryFn: async () => {
      if (isGo) {
        // Scheduler compile-status contract (dataset-level, variant-agnostic).
        // The status is NOT a task percentage: we carry the raw state, the
        // MySQL inflight/backlog entry counts and the error diagnostic, and
        // derive the display status in useGenerateStatus. progress is only set
        // so the shared refetch/status helpers keep their contract (idle->0,
        // running/pending->0, completed->1, error->0).
        const res = await getDatasetCompilationStatus(id!);
        const data = res?.data;
        // The handler returns HTTP 200 with a non-zero business code for
        // authorization/business errors (e.g. "no authorization"). The request
        // interceptor only shows a toast and does not reject, so without this
        // explicit check a failed read would be mapped to a misleading idle
        // state. Reject so the query surfaces the error instead.
        if (!data || data.code !== 0) {
          throw new Error(data?.message || 'Failed to read compilation status');
        }
        const st = data.data ?? {};
        const state: string = st.state ?? 'idle';
        const error: string = st.error ?? '';
        return {
          progress: state === 'completed' ? 1 : (st.progress ?? 0),
          progress_msg: st.progress_msg || error || state,
          compilationState: state,
          currentPhase: st.current_phase ?? '',
          inflight: st.inflight ?? 0,
          backlog: st.backlog ?? 0,
          compilationError: error,
        } as ITraceInfo;
      }
      const { data } = await traceIndex(id!, traceType);
      return data?.data ?? {};
    },
  });
};

const TraceTypeMap: Record<GenerateType, TraceType> = {
  [GenerateType.KnowledgeGraph]: TraceType.Graph,
  [GenerateType.Raptor]: TraceType.Raptor,
  [GenerateType.Artifact]: TraceType.Artifact,
  [GenerateType.ToSkills]: TraceType.Skill,
  [GenerateType.MindMap]: TraceType.MindMap,
  [GenerateType.Timeline]: TraceType.Timeline,
  [GenerateType.SessionEssence]: TraceType.SessionEssence,
  [GenerateType.SessionGraph]: TraceType.SessionGraph,
};

export const useTraceRunData = (type: GenerateType) => {
  const { id } = useParams();
  return useTraceQuery(type, TraceTypeMap[type], true, id);
};

export const useUnBindTask = () => {
  const { id } = useParams();
  const { t } = useTranslation();

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
  const { t } = useTranslation();
  const isGo = useIsGoBackend();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [DatasetKey.generate],
    mutationFn: async ({ type }: { type: GenerateType }) => {
      // Go: dataset compilation is driven automatically by the scheduler
      // on document completion; there is no manual RunIndex trigger, so a manual
      // "generate" must NOT pretend to succeed. The UI hides/disables the
      // control; if it is ever invoked, reject loudly so callers don't mistake
      // it for a real run (plan v4.1 §4.2).
      if (isGo) {
        throw new Error(t('message.compileNotSupported'));
      }
      const { data } = await runIndex(id!, TraceTypeMap[type]);
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
      // Go: the scheduler has no task-level cancel; dataset compilation
      // is auto-driven. There is no pause to perform, so reject rather than
      // report success (the UI hides the pause control — plan v4.1 §4.2).
      if (isGo) {
        throw new Error(t('message.compileNotSupported'));
      }
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

export function useGenerateStatus(data?: ITraceInfo) {
  const isGo = useIsGoBackend();
  const status = useMemo(() => {
    if (!data) {
      return GenerateStatus.Start;
    }
    if (isGo) {
      // Go: derive from the scheduler contract, not a fake task
      // percentage. Error diagnostic takes priority; otherwise map the raw
      // dataset-level state (completed->Completed, idle->Start, running/pending
      // ->Running).
      if (data.compilationError) {
        return GenerateStatus.Failed;
      }
      const st = data.compilationState;
      if (st === 'completed') return GenerateStatus.Completed;
      if (st === 'running' || st === 'pending') return GenerateStatus.Running;
      return GenerateStatus.Start;
    }
    if (data.progress >= 1) {
      return GenerateStatus.Completed;
    } else if (!data.progress && data.progress !== 0) {
      return GenerateStatus.Start;
    } else if (data.progress < 0) {
      return GenerateStatus.Failed;
    } else if (data.progress < 1) {
      return GenerateStatus.Running;
    }
    return GenerateStatus.Start;
  }, [data, isGo]);

  const percent = useMemo(() => {
    if (isGo) {
      return status === GenerateStatus.Failed
        ? 100
        : (data?.progress ?? 0) * 100;
    }
    if (status === GenerateStatus.Failed) {
      return 100;
    } else if (status === GenerateStatus.Running) {
      return data!.progress * 100;
    }
    return 0;
  }, [status, data, isGo]);

  return { status, percent };
}
