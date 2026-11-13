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

import {
  IActionBinding,
  IActionCallResult,
  IActionContractEditList,
  IActionExecutionFilters,
  IActionExecutionList,
  IModelAction,
} from '@/interfaces/database/compilation-model';
import {
  deleteActionBinding,
  dryRunAction,
  executeAction,
  listActionBindings,
  listActionContractEdits,
  listActionExecutions,
  listModelActions,
  saveActionBinding,
  updateActionContract,
} from '@/services/compilation-model-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

export const enum OntologyActionApiAction {
  FetchActions = 'fetchModelActions',
  FetchBindings = 'fetchActionBindings',
  FetchExecutions = 'fetchActionExecutions',
  FetchContractEdits = 'fetchActionContractEdits',
  UpdateContract = 'updateActionContract',
  SaveBinding = 'saveActionBinding',
  DeleteBinding = 'deleteActionBinding',
  DryRun = 'dryRunAction',
  Execute = 'executeAction',
}

export const OntologyActionKeys = {
  /**
   * Actions, bindings and the verdict all describe one row on screen, so a write
   * to any of them invalidates all three: a saved binding changes the verdict of
   * an action that did not itself change.
   */
  all: (id?: string) => [OntologyActionApiAction.FetchActions, id] as const,
  actions: (id?: string) => [OntologyActionApiAction.FetchActions, id] as const,
  bindings: (id?: string) =>
    [OntologyActionApiAction.FetchBindings, id] as const,
  executions: (id?: string, page?: number, filters?: IActionExecutionFilters) =>
    [OntologyActionApiAction.FetchExecutions, id, page, filters] as const,
  edits: (id?: string, actionName?: string) =>
    [OntologyActionApiAction.FetchContractEdits, id, actionName] as const,
};

export const useFetchModelActions = (id?: string, enabled = true) => {
  const {
    data,
    isFetching: loading,
    error,
  } = useQuery<IModelAction[]>({
    queryKey: OntologyActionKeys.actions(id),
    enabled: !!id && enabled,
    queryFn: async () => {
      const { data } = await listModelActions(id!);
      return (data?.data?.actions ?? []) as IModelAction[];
    },
  });

  return { actions: data, loading, error };
};

export const useFetchActionBindings = (id?: string, enabled = true) => {
  const {
    data,
    isFetching: loading,
    error,
  } = useQuery<{
    bindings: IActionBinding[];
    total: number;
    exec_enabled: boolean;
  }>({
    queryKey: OntologyActionKeys.bindings(id),
    enabled: !!id && enabled,
    queryFn: async () => {
      const { data } = await listActionBindings(id!);
      return {
        bindings: (data?.data?.bindings ?? []) as IActionBinding[],
        total: data?.data?.total ?? 0,
        // The process switch is reported by the server so the page can explain
        // why nothing is runnable without the operator reading a config file.
        exec_enabled: !!data?.data?.exec_enabled,
      };
    },
  });

  return {
    bindings: data?.bindings,
    execEnabled: data?.exec_enabled,
    loading,
    error,
  };
};

export const useFetchActionExecutions = (
  id?: string,
  page = 1,
  enabled = true,
  /**
   * Filters go to the server. Narrowing the page that happens to be loaded would
   * answer "among these rows", while the reader is asking about the model.
   */
  filters?: IActionExecutionFilters,
  pageSize = 20,
) => {
  const {
    data,
    isFetching: loading,
    error,
  } = useQuery<IActionExecutionList>({
    queryKey: OntologyActionKeys.executions(id, page, filters),
    enabled: !!id && enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listActionExecutions(id!, {
        page,
        page_size: pageSize,
        action_name: filters?.action_name || undefined,
        verdict: filters?.verdict || undefined,
        status: filters?.status || undefined,
        mode: filters?.mode || undefined,
        from: filters?.from || undefined,
        to: filters?.to || undefined,
      });
      return (data?.data ?? {
        executions: [],
        total: 0,
      }) as IActionExecutionList;
    },
  });

  return { executions: data?.executions, total: data?.total, loading, error };
};

/**
 * The manual edit history of one action.
 *
 * Fetched per action on demand: the workbench shows it under the contract of the
 * action the reader picked, and loading every action's history at once would be
 * a request paid for rows nobody is looking at.
 */
export const useFetchActionContractEdits = (
  id?: string,
  actionName?: string,
  enabled = true,
) => {
  const { data, isFetching: loading } = useQuery<IActionContractEditList>({
    queryKey: OntologyActionKeys.edits(id, actionName),
    enabled: !!id && !!actionName && enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listActionContractEdits(id!, actionName!, {
        page: 1,
        page_size: 20,
      });
      return (data?.data ?? { edits: [], total: 0 }) as IActionContractEditList;
    },
  });

  return { edits: data?.edits, total: data?.total, loading };
};

export const useUpdateActionContract = () => {
  const queryClient = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyActionApiAction.UpdateContract],
    mutationFn: async ({
      id,
      actionId,
      datasetId,
      contract,
      reason,
    }: {
      id: string;
      actionId: string;
      datasetId?: string;
      contract: Record<string, unknown>;
      reason: string;
    }) => {
      const { data } = await updateActionContract(
        id,
        actionId,
        { contract, reason },
        datasetId ? { dataset_id: datasetId } : undefined,
      );
      return data;
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: OntologyActionKeys.actions(variables.id),
      });
      // The review queue shows the same contract, so it goes stale too.
      queryClient.invalidateQueries({
        queryKey: ['reviewAssertions', variables.id],
      });
    },
  });

  return { loading, updateContract: mutateAsync };
};

export const useSaveActionBinding = () => {
  const queryClient = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyActionApiAction.SaveBinding],
    mutationFn: async ({
      id,
      actionName,
      payload,
    }: {
      id: string;
      actionName: string;
      payload: Parameters<typeof saveActionBinding>[2];
    }) => {
      const { data } = await saveActionBinding(id, actionName, payload);
      return data;
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: OntologyActionKeys.actions(variables.id),
      });
      queryClient.invalidateQueries({
        queryKey: OntologyActionKeys.bindings(variables.id),
      });
    },
  });

  return { loading, saveBinding: mutateAsync };
};

export const useDeleteActionBinding = () => {
  const queryClient = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyActionApiAction.DeleteBinding],
    mutationFn: async ({
      id,
      actionName,
    }: {
      id: string;
      actionName: string;
    }) => {
      const { data } = await deleteActionBinding(id, actionName);
      return data;
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: OntologyActionKeys.actions(variables.id),
      });
      queryClient.invalidateQueries({
        queryKey: OntologyActionKeys.bindings(variables.id),
      });
    },
  });

  return { loading, deleteBinding: mutateAsync };
};

export const useDryRunAction = () => {
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyActionApiAction.DryRun],
    mutationFn: async ({
      id,
      actionName,
      args,
    }: {
      id: string;
      actionName: string;
      args?: Record<string, unknown>;
    }): Promise<IActionCallResult> => {
      const { data } = await dryRunAction(id, actionName, { arguments: args });
      return data?.data as IActionCallResult;
    },
  });

  return { loading, dryRun: mutateAsync };
};

export const useExecuteAction = () => {
  const queryClient = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyActionApiAction.Execute],
    mutationFn: async ({
      id,
      actionName,
      args,
      reason,
    }: {
      id: string;
      actionName: string;
      args?: Record<string, unknown>;
      reason: string;
    }): Promise<IActionCallResult> => {
      const { data } = await executeAction(id, actionName, {
        arguments: args,
        reason,
      });
      return data?.data as IActionCallResult;
    },
    onSuccess: (_data, variables) => {
      // Every attempt is audited, refused ones included, so the trail is stale
      // after any call — not only after a successful one.
      queryClient.invalidateQueries({
        queryKey: [OntologyActionApiAction.FetchExecutions, variables.id],
      });
    },
  });

  return { loading, execute: mutateAsync };
};
