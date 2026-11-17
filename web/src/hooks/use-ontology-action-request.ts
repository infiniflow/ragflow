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
  IModelMCPTool,
} from '@/interfaces/database/ontology-model';
import {
  deleteActionBinding,
  executeAction,
  getModelMCPServers,
  listActionBindings,
  listActionContractEdits,
  listActionExecutions,
  listModelActions,
  listModelMCPTools,
  saveActionBinding,
  updateActionContract,
  updateModelMCPServers,
} from '@/services/ontology-model-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

export const enum OntologyActionApiAction {
  FetchActions = 'fetchModelActions',
  FetchBindings = 'fetchActionBindings',
  FetchMCPEndpoint = 'fetchModelMCPEndpoint',
  FetchMCPTools = 'fetchModelMCPTools',
  UpdateMCPEndpoint = 'updateModelMCPEndpoint',
  FetchExecutions = 'fetchActionExecutions',
  FetchContractEdits = 'fetchActionContractEdits',
  UpdateContract = 'updateActionContract',
  SaveBinding = 'saveActionBinding',
  DeleteBinding = 'deleteActionBinding',
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
  mcpEndpoint: (id?: string) =>
    [OntologyActionApiAction.FetchMCPEndpoint, id] as const,
  mcpTools: (id?: string, server?: string) =>
    [OntologyActionApiAction.FetchMCPTools, id, server] as const,
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

/**
 * The model-level MCP addresses.
 *
 * Kept apart from the bindings query because they are not bindings: they are the
 * systems this model's actions may reach, and the page shows them above the list.
 * The first entry is the default a binding follows.
 */
export const useFetchModelMCPServers = (id?: string, enabled = true) => {
  const { data, isFetching: loading } = useQuery<string[]>({
    queryKey: OntologyActionKeys.mcpEndpoint(id),
    enabled: !!id && enabled,
    queryFn: async () => {
      const { data } = await getModelMCPServers(id!);
      return (data?.data?.mcp_servers ?? []) as string[];
    },
  });

  return { servers: data ?? [], loading };
};

/**
 * The tool names one of the model's addresses exposes.
 *
 * Fetched when the run test opens instead of on page load: the call reaches the
 * MCP server, and paying for it on every visit to the actions page would slow
 * the list down for rows nobody is binding. `server` is the address the binding
 * will dial, so the picker lists that server's tools rather than the default's.
 */
export const useFetchModelMCPTools = (
  id?: string,
  server?: string,
  enabled = true,
) => {
  const {
    data,
    isFetching: loading,
    error,
    refetch,
  } = useQuery<IModelMCPTool[]>({
    queryKey: OntologyActionKeys.mcpTools(id, server),
    enabled: !!id && enabled,
    staleTime: 0,
    gcTime: 0,
    retry: false,
    queryFn: async () => {
      const { data } = await listModelMCPTools(id!, server);
      if (data?.code !== 0) {
        // A failed listing is not "the server exposes nothing": it means the
        // address could not be reached or was refused. Raising it keeps the
        // picker from claiming an empty tool list.
        const failure = new Error(
          data?.message || 'The tool list could not be read.',
        ) as Error & { code?: string };
        failure.code = data?.data?.code;
        throw failure;
      }
      return (data?.data?.tools ?? []) as IModelMCPTool[];
    },
  });

  return { tools: data, loading, error, refetch };
};

export const useUpdateModelMCPServers = () => {
  const queryClient = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyActionApiAction.UpdateMCPEndpoint],
    mutationFn: async ({ id, servers }: { id: string; servers: string[] }) => {
      const { data } = await updateModelMCPServers(id, servers);
      return data;
    },
    // Await the refetch before the caller clears its draft: the editor renders
    // the fetched value once it stops editing, so resolving first left a beat
    // where the field showed the previous (empty) address — indistinguishable
    // from the value having been dropped.
    onSuccess: async (_data, variables) => {
      // The address decides whether a tool is reachable, so every verdict on the
      // page is stale after this write.
      await queryClient.invalidateQueries({
        queryKey: OntologyActionKeys.mcpEndpoint(variables.id),
      });
      queryClient.invalidateQueries({
        queryKey: OntologyActionKeys.actions(variables.id),
      });
      queryClient.invalidateQueries({
        queryKey: OntologyActionKeys.bindings(variables.id),
      });
    },
  });

  return { loading, updateEndpoints: mutateAsync };
};

export const useSaveActionBinding = () => {
  const queryClient = useQueryClient();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [OntologyActionApiAction.SaveBinding],
    mutationFn: async ({
      id,
      actionName,
      payload,
      bindingId,
    }: {
      id: string;
      actionName: string;
      payload: Parameters<typeof saveActionBinding>[2];
      /** Existing row to replace; omitted for the first binding of an action. */
      bindingId?: string;
    }) => {
      const { data } = await saveActionBinding(
        id,
        actionName,
        payload,
        bindingId ? { binding_id: bindingId } : undefined,
      );
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
      bindingId,
    }: {
      id: string;
      actionName: string;
      bindingId: string;
    }) => {
      const { data } = await deleteActionBinding(id, actionName, {
        binding_id: bindingId,
      });
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
      if (data?.code !== 0) {
        // A refused call is answered with the verdict and its reasons instead of
        // a result; rendering that as "no result" hid the explanation the run
        // test exists to show.
        return {
          action_name: actionName,
          verdict: (data?.data?.verdict ??
            'refuse') as IActionCallResult['verdict'],
          reasons: data?.data?.reasons ?? [],
          status: 'refused',
          error: data?.message,
        } satisfies IActionCallResult;
      }
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
