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

import { FileUploadProps } from '@/components/file-upload';
import { useHandleFilterSubmit } from '@/components/list-filter-bar/use-handle-filter-submit';
import message from '@/components/ui/message';
import { AgentCategory, AgentGlobals } from '@/constants/agent';
import { ListDeletionKey } from '@/constants/list-deletion';
import { useFetchTenantInfo } from '@/hooks/use-user-setting-request';
import {
  AgentListItem,
  DSL,
  IAgentLogResponse,
  IAgentLogsRequest,
  IAgentLogsResponse,
  IBuiltinPipelineListResponse,
  IFlow,
  IFlowTemplate,
  IAgentWebhookCredential,
  IPipeLineListRequest,
  ITraceData,
  IWebhookTrace,
} from '@/interfaces/database/agent';
import {
  IAgentWebhookTraceRequest,
  IDebugSingleRequestBody,
} from '@/interfaces/request/agent';
import i18n from '@/locales/config';
import { IInputs } from '@/pages/agent/interface';
import { useGetSharedChatSearchParams } from '@/pages/next-chats/hooks/use-send-shared-message';
import agentService, {
  createAgentSession,
  deleteAgentSession,
  fetchAgentLogsByCanvasId,
  fetchAgentLogsById,
  fetchPipeLineList,
  fetchSharedTrace,
  fetchTrace,
  fetchWebhookTrace,
  issueAgentWebhookCredential,
  revokeAgentWebhookCredential,
  updateAgent,
  updateAgentTags,
  updateVersion,
  uploadAgentFile,
} from '@/services/agent-service';
import { buildMessageListWithUuid } from '@/utils/chat';
import { markListItemsDeleted } from '@/utils/list-deletion-util';
import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { get, isEmpty, set } from 'lodash';
import { useCallback, useState } from 'react';
import { useParams, useSearchParams } from 'react-router';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import { useSessionList } from './use-session-list';

export const enum AgentApiAction {
  FetchAgentListByPage = 'fetchAgentListByPage',
  FetchAllAgentList = 'fetchAllAgentList',
  FetchAgentList = 'fetchAgentList',
  FetchAgentFilters = 'fetchAgentFilters',
  UpdateAgentSetting = 'updateAgentSetting',
  DeleteAgent = 'deleteAgent',
  FetchAgentDetail = 'fetchAgentDetail',
  ResetAgent = 'resetAgent',
  CreateAgent = 'createAgent',
  SaveAgent = 'saveAgent',
  FetchAgentTemplates = 'fetchAgentTemplates',
  UploadAgentFile = 'uploadAgentFile',
  UploadAgentFileWithProgress = 'uploadAgentFileWithProgress',
  Trace = 'trace',
  TestDbConnect = 'testDbConnect',
  DebugSingle = 'debugSingle',
  FetchInputForm = 'fetchInputForm',
  FetchVersionList = 'fetchVersionList',
  FetchVersion = 'fetchVersion',
  FetchExternalAgentInputs = 'fetchExternalAgentInputs',
  SetAgentSetting = 'setAgentSetting',
  FetchPrompt = 'fetchPrompt',
  CancelDataflow = 'cancelDataflow',
  CancelCanvas = 'cancelCanvas',
  FetchWebhookTrace = 'fetchWebhookTrace',
  CreateWebhookCredential = 'createWebhookCredential',
  RevokeWebhookCredential = 'revokeWebhookCredential',
  FetchSessionsByCanvasId = 'fetchSessionsByCanvasId',
  FetchSessionById = 'fetchSessionById',
  CreateAgentSession = 'createAgentSession',
  DeleteAgentSession = 'deleteAgentSession',
  FetchSessionByIdManually = 'fetchSessionByIdManually',
  FetchAgentLog = 'fetchAgentLog',
  FetchSharedAgent = 'fetchSharedAgent',
  FetchAgentTags = 'fetchAgentTags',
  UpdateAgentTags = 'updateAgentTags',
  FetchPipelineNodes = 'fetchPipelineNodes',
  FetchBuiltinPipelineList = 'fetchBuiltinPipelineList',
  FetchBuiltinPipelineDetail = 'fetchBuiltinPipelineDetail',
}

// Invalidation rule for the list-page queries (list/filters/tags): they all
// use gcTime: 0 and refetch on mount, so invalidating them only matters while
// the agents list page is still mounted — i.e. from the list page's own
// mutations (delete, rename, tag edits, compilation group delete). Create and
// save flows navigate away or run on the canvas page, where these queries are
// inactive; invalidating them there only fires wasted refetches from the list
// page still mounted behind the create dialog.
export const AgentKeys = {
  templates: () => [AgentApiAction.FetchAgentTemplates] as const,
  list: (params?: unknown) =>
    params === undefined
      ? ([AgentApiAction.FetchAgentListByPage] as const)
      : ([AgentApiAction.FetchAgentListByPage, params] as const),
  all: () => [AgentApiAction.FetchAllAgentList] as const,
  listAll: (canvasCategory?: string) =>
    [AgentApiAction.FetchAgentList, canvasCategory] as const,
  filters: (canvasCategory?: string) =>
    canvasCategory === undefined
      ? ([AgentApiAction.FetchAgentFilters] as const)
      : ([AgentApiAction.FetchAgentFilters, canvasCategory] as const),
  tags: (canvasCategory?: string) =>
    canvasCategory === undefined
      ? ([AgentApiAction.FetchAgentTags] as const)
      : ([AgentApiAction.FetchAgentTags, canvasCategory] as const),
  detail: (agentId?: string) =>
    [AgentApiAction.FetchAgentDetail, agentId] as const,
  inputForm: (agentId?: string, componentId?: string) =>
    [AgentApiAction.FetchInputForm, agentId, componentId] as const,
  versionList: (agentId?: string) =>
    [AgentApiAction.FetchVersionList, agentId] as const,
  version: (agentId?: string, versionId?: string) =>
    [AgentApiAction.FetchVersion, agentId, versionId] as const,
};

export const useFetchAgentTemplates = () => {
  const { data } = useQuery<IFlowTemplate[]>({
    queryKey: AgentKeys.templates(),
    initialData: [],
    queryFn: async () => {
      const { data } = await agentService.listAgentTemplate();

      return data.data;
    },
  });

  return data;
};

const buildAgentListParams = ({
  page,
  pageSize,
  keywords,
  canvasCategory,
  ownerIds,
  tags,
}: {
  page: number;
  pageSize: number;
  keywords?: string;
  canvasCategory?: string;
  ownerIds?: string[];
  tags?: string[];
}) => {
  const params: Record<string, unknown> = {
    page,
    page_size: pageSize,
  };

  if (keywords) {
    params.keywords = keywords;
  }
  if (canvasCategory) {
    params.canvas_category = canvasCategory;
  }
  if (Array.isArray(ownerIds) && ownerIds.length > 0) {
    params.owner_ids = ownerIds.join(',');
  }
  if (Array.isArray(tags) && tags.length > 0) {
    params.tags = tags.join(',');
  }

  return params;
};

export const useFetchAgentListByPage = (canvasCategory?: string) => {
  const { searchString, setSearchString, handleInputChange } =
    useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });
  const { filterValue, setFilterValue, handleFilterSubmit, checkValue } =
    useHandleFilterSubmit();
  const owner = filterValue.owner;
  const tags = Array.isArray(filterValue.tags) ? filterValue.tags : undefined;

  const requestParams = buildAgentListParams({
    page: pagination.current,
    pageSize: pagination.pageSize,
    keywords: debouncedSearchString,
    canvasCategory,
    ownerIds: Array.isArray(owner) ? owner : undefined,
    tags,
  });

  const { data, isFetching: loading } = useQuery<{
    canvas: AgentListItem[];
    total: number;
  }>({
    queryKey: AgentKeys.list({
      debouncedSearchString,
      ...pagination,
      canvasCategory,
      filterValue,
    }),
    placeholderData: (previousData) => {
      if (previousData === undefined) {
        return { canvas: [], total: 0 };
      }
      return previousData;
    },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.listAgents(
        {
          params: requestParams,
        },
        true,
      );

      return data?.data;
    },
  });

  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      // setPagination({ page: 1 });
      handleInputChange(e);
    },
    [handleInputChange],
  );

  return {
    data: data?.canvas ?? [],
    loading,
    searchString,
    debouncedSearchString,
    setSearchString,
    handleInputChange: onInputChange,
    pagination: { ...pagination, total: data?.total ?? 0 },
    setPagination,
    filterValue,
    setFilterValue,
    handleFilterSubmit,
    checkValue,
  };
};

export const fetchAllAgents = async (): Promise<AgentListItem[]> => {
  const all: AgentListItem[] = [];
  let page = 1;
  let total = Number.POSITIVE_INFINITY;
  while (all.length < total) {
    const { data } = await agentService.listAgents(
      {
        params: buildAgentListParams({
          page,
          pageSize: 100,
          canvasCategory: AgentCategory.AgentCanvas,
        }),
      },
      true,
    );
    const canvas: AgentListItem[] = data?.data?.canvas ?? [];
    all.push(...canvas);
    total = data?.data?.total ?? all.length;
    if (canvas.length === 0) {
      break;
    }
    page += 1;
  }
  return all;
};

export function useFetchAllAgentList() {
  const { data, isFetching: loading } = useQuery<AgentListItem[]>({
    queryKey: AgentKeys.all(),
    queryFn: fetchAllAgents,
  });

  return { data, loading };
}

export const useUpdateAgentSetting = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.UpdateAgentSetting],
    mutationFn: async (params: any) => {
      const ret = await updateAgent(params.id, {
        title: params.title,
        description: params.description,
        permission: params.permission,
        avatar: params.avatar,
      });
      if (ret?.data?.code === 0) {
        message.success('success');
        queryClient.invalidateQueries({
          queryKey: AgentKeys.list(),
        });
      }
      return ret?.data?.code;
    },
  });

  return { data, loading, updateAgentSetting: mutateAsync };
};

export const useDeleteAgent = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.DeleteAgent],
    mutationFn: async (agentId: string) => {
      const { data } = await agentService.deleteAgent(agentId);
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: AgentKeys.list(),
        });
        queryClient.invalidateQueries({
          queryKey: AgentKeys.filters(),
        });
        queryClient.invalidateQueries({
          queryKey: AgentKeys.tags(),
        });
        markListItemsDeleted(ListDeletionKey.AgentList);
      }
      return data?.data ?? false;
    },
  });

  return { data, loading, deleteAgent: mutateAsync };
};

export interface IAgentTagCount {
  tag: string;
  count: number;
}

export const useFetchAgentTags = (canvasCategory?: string) => {
  const { data, isFetching: loading } = useQuery<IAgentTagCount[]>({
    queryKey: AgentKeys.tags(canvasCategory),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.listAgentTags(
        {
          params: canvasCategory ? { canvas_category: canvasCategory } : {},
        },
        true,
      );
      return data?.data ?? [];
    },
  });
  return { data, loading };
};

export const useUpdateAgentTags = () => {
  const queryClient = useQueryClient();
  const { isPending: loading, mutateAsync } = useMutation({
    mutationKey: [AgentApiAction.UpdateAgentTags],
    mutationFn: async ({
      agentId,
      tags,
    }: {
      agentId: string;
      tags: string[];
    }) => {
      const { data } = await updateAgentTags(agentId, tags);
      if (data?.code === 0) {
        queryClient.invalidateQueries({
          queryKey: AgentKeys.list(),
        });
        queryClient.invalidateQueries({
          queryKey: AgentKeys.tags(),
        });
      } else {
        message.error(data?.message || 'Update failed');
      }
      return data?.code === 0;
    },
  });
  return { loading, updateAgentTags: mutateAsync };
};

export const useFetchAgent = (): {
  data: IFlow;
  loading: boolean;
  refetch: () => void;
} => {
  const { id } = useParams();
  const { sharedId } = useGetSharedChatSearchParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery({
    queryKey: AgentKeys.detail(sharedId || id),
    initialData: {} as IFlow,
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.getAgent(sharedId || id);

      const messageList = buildMessageListWithUuid(
        get(data, 'data.dsl.messages', []),
      );
      set(data, 'data.dsl.messages', messageList);

      const sysHistoryPath = [
        'data',
        'dsl',
        'globals',
        AgentGlobals.SysHistory,
      ];

      if (isEmpty(get(data, sysHistoryPath))) {
        set(data, sysHistoryPath, []);
      }

      return data?.data ?? {};
    },
  });

  return { data, loading, refetch };
};

export const useResetAgent = () => {
  const { id } = useParams();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.ResetAgent],
    mutationFn: async () => {
      const { data } = await agentService.resetAgent(id);
      return data;
    },
  });

  return { data, loading, resetAgent: mutateAsync };
};

// Every create flow (blank, template, JSON import, built-in pipeline copy)
// navigates to the new canvas on success, and the list-page queries refetch on
// remount (gcTime: 0) — so creation intentionally invalidates nothing.
export const useCreateAgent = (showMessage: boolean = true) => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.CreateAgent],
    mutationFn: async (params: {
      title?: string;
      dsl?: DSL | Record<string, any>;
      avatar?: string;
      canvas_category?: string;
    }) => {
      const { data = {} } = await agentService.createAgent(params);
      if (data.code === 0 && showMessage) {
        message.success(i18n.t('message.created'));
      }
      return data;
    },
  });

  return { data, loading, createAgent: mutateAsync };
};

export const useSaveAgent = (
  showMessage: boolean = true,
  skipDetailRefresh: boolean = false,
) => {
  const { id } = useParams();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.SaveAgent],
    mutationFn: async (params: {
      id?: string;
      title?: string;
      dsl?: DSL | Record<string, any>;
      avatar?: string | null;
      release?: string;
      description?: string | null;
      permission?: string;
      autosave?: boolean;
      alias?: string;
    }) => {
      const agentId = params.id ?? id;
      if (!agentId) {
        return {};
      }
      const { data = {} } = await updateAgent(agentId, {
        title: params.title,
        dsl: params.dsl,
        avatar: params.avatar,
        description: params.description,
        permission: params.permission,
        release: params.release,
        autosave: params.autosave,
        alias: params.alias,
      });
      if (data.code === 0) {
        if (showMessage) {
          message.success(i18n.t('message.modified'));
        }
        // Every save can prune the oldest unpublished versions server-side,
        // so the version history list must refetch even when autosave skips
        // the detail refresh.
        queryClient.invalidateQueries({
          queryKey: AgentKeys.versionList(agentId),
        });
        if (!skipDetailRefresh) {
          queryClient.invalidateQueries({
            queryKey: AgentKeys.detail(agentId),
          });
        }
      }
      return data;
    },
  });

  return { data, loading, saveAgent: mutateAsync };
};

// Only one file can be uploaded at a time
export const useUploadAgentFile = () => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const shared_id = searchParams.get('shared_id');
  const agentId = id || shared_id;
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.UploadAgentFile],
    mutationFn: async (body: any) => {
      let nextBody = body;
      try {
        if (Array.isArray(body)) {
          nextBody = new FormData();
          body.forEach((file: File) => {
            nextBody.append('file', file as any);
          });
        }

        const { data } = await uploadAgentFile(agentId as string, nextBody);
        if (data?.code === 0) {
          message.success(i18n.t('message.uploaded'));
        }
        return data;
      } catch (error) {
        message.error(error as string);
      }
    },
  });

  return { data, loading, uploadAgentFile: mutateAsync };
};

export const useUploadAgentFileWithProgress = (identifier?: string | null) => {
  const { id } = useParams();

  type UploadParameters = Parameters<NonNullable<FileUploadProps['onUpload']>>;

  type X = { files: UploadParameters[0]; options: UploadParameters[1] };

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.UploadAgentFileWithProgress],
    mutationFn: async ({
      files,
      options: { onError, onSuccess, onProgress },
    }: X) => {
      const formData = new FormData();
      try {
        if (Array.isArray(files)) {
          files.forEach((file: File) => {
            formData.append('file', file);
          });
        }

        const { data } = await agentService.uploadAgentFile(
          {
            agentId: identifier || id,
            data: formData,
            onUploadProgress: ({ progress }) => {
              files.forEach((file) => {
                onProgress(file, (progress || 0) * 100);
              });
            },
          },
          true,
        );
        if (data?.code === 0) {
          files.forEach((file) => {
            onSuccess(file);
          });
          message.success(i18n.t('message.uploaded'));
        }
        return data;
      } catch (error) {
        files.forEach((file) => {
          onError(file, error as Error);
        });
        message.error((error as Error)?.message || 'Upload failed');
      }
    },
  });

  return { data, loading, uploadAgentFile: mutateAsync };
};

export const useFetchMessageTrace = (canvasId?: string, isShare?: boolean) => {
  const { id } = useParams();
  // In shared mode there's no :id route param and `canvasId` actually carries
  // the share (beta) APIToken — route through fetchSharedTrace so the request
  // hits the beta-token-aware endpoint instead of /agents/<id>/logs which
  // requires a session login (fixes #14985).
  const queryId = id || canvasId;
  const [messageId, setMessageId] = useState('');
  const [isStopFetchTrace, setISStopFetchTrace] = useState(false);

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<ITraceData[]>({
    queryKey: [AgentApiAction.Trace, queryId, messageId, !!isShare],
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    enabled: !!queryId && !!messageId,
    refetchInterval: !isStopFetchTrace ? 3000 : false,
    queryFn: async () => {
      const { data } = isShare
        ? await fetchSharedTrace({
            shared_id: queryId as string,
            message_id: messageId,
          })
        : await fetchTrace({
            canvas_id: queryId as string,
            message_id: messageId,
          });

      return Array.isArray(data?.data) ? data?.data : [];
    },
  });

  return {
    data,
    loading,
    refetch,
    setMessageId,
    messageId,
    isStopFetchTrace,
    setISStopFetchTrace,
  };
};

export const useTestDbConnect = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.TestDbConnect],
    mutationFn: async (params: any) => {
      const ret = await agentService.testDbConnect(params);
      if (ret?.data?.code === 0) {
        const successMessage = ret?.data?.message;
        message.success(
          typeof successMessage === 'string' &&
            successMessage.trim() &&
            successMessage.trim().toLowerCase() !== 'success'
            ? successMessage
            : i18n.t('setting.dataSourceTestSuccess'),
        );
      }
      return ret;
    },
  });

  return { data, loading, testDbConnect: mutateAsync };
};

export const useDebugSingle = () => {
  const { id } = useParams();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.DebugSingle],
    mutationFn: async (params: IDebugSingleRequestBody) => {
      const ret = await agentService.debugSingle(
        {
          agentId: id as string,
          componentId: params.component_id,
          data: {
            params: params.params,
          },
        },
        true,
      );
      if (ret?.data?.code !== 0) {
        message.error(ret?.data?.message);
      }
      return ret?.data?.data;
    },
  });

  return { data, loading, debugSingle: mutateAsync };
};

export const useFetchInputForm = (componentId?: string) => {
  const { id } = useParams();

  const { data } = useQuery<Record<string, any>>({
    queryKey: AgentKeys.inputForm(id, componentId),
    initialData: {},
    enabled: !!id && !!componentId,
    queryFn: async () => {
      const { data } = await agentService.inputForm(
        { agentId: id as string, componentId: componentId as string },
        true,
      );

      return data.data;
    },
  });

  return data;
};

export const useFetchVersionList = () => {
  const { id } = useParams();
  const { data, isFetching: loading } = useQuery<
    Array<{
      created_at: string;
      title: string;
      id: string;
      release?: boolean;
      alias?: string;
    }>
  >({
    queryKey: AgentKeys.versionList(id),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.fetchVersionList(id);

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

// Row shape of a published canvas version, newest-first (design section 7/E1),
// including the free-text alias (A2/F1).
export interface ICanvasVersionRow {
  id: string;
  title: string;
  created_at: string;
  release?: boolean;
  alias?: string;
}

// Shared query key/options for a canvas's published version list. The raw
// 'canvasVersionList' key predates the AgentKeys factory; both the single- and
// multi-canvas hooks must keep using it verbatim so the version delete/alias
// mutations below invalidate every observer.
const canvasVersionListQuery = (canvasId: string) => ({
  queryKey: ['canvasVersionList', canvasId] as const,
  queryFn: async (): Promise<ICanvasVersionRow[]> => {
    const { data } = await agentService.fetchVersionList(canvasId);
    return data?.data ?? [];
  },
});

// useFetchCanvasVersionList lists the published versions of an arbitrary canvas
// (dataflow or agent) by id. Unlike useFetchVersionList it takes the canvas id
// explicitly (not from route params) so the document pipeline dialog can load
// versions for the currently selected dataflow canvas.
export const useFetchCanvasVersionList = (canvasId?: string) => {
  const {
    data,
    isFetching: loading,
    isError,
  } = useQuery({
    queryKey: ['canvasVersionList', canvasId],
    initialData: [],
    gcTime: 0,
    enabled: !!canvasId,
    queryFn: async (): Promise<ICanvasVersionRow[]> => {
      if (!canvasId) return [];
      const { data } = await agentService.fetchVersionList(canvasId);
      return data?.data ?? [];
    },
  });

  return { data, loading, isError };
};

// useFetchCanvasVersions lazily loads the published versions of several
// canvases at once (the parser tree select fetches a canvas's versions only
// when the user expands it). Unlike useFetchCanvasVersionList it sets no
// initialData, so `isSuccess` distinguishes "loaded and empty" from "still
// loading". The returned array aligns index-by-index with `canvasIds`.
export const useFetchCanvasVersions = (canvasIds: string[]) => {
  return useQueries({
    queries: canvasIds.map((canvasId) => ({
      ...canvasVersionListQuery(canvasId),
      gcTime: 0,
    })),
  });
};

// useDeleteVersion removes a single canvas version (design section 7/B2). The
// backend still allows deletion when the version is referenced by a dataset or
// file; the binding then enters a Missing (unparseable) state, so the UI shows a
// confirmation that warns about breaking bound files (B2.1).
export const useDeleteVersion = () => {
  const queryClient = useQueryClient();
  const {
    isPending: loading,
    isError,
    mutateAsync,
  } = useMutation({
    mutationFn: async ({
      canvasId,
      versionId,
    }: {
      canvasId: string;
      versionId: string;
    }) => {
      const { data } = await agentService.deleteVersion({
        agentId: canvasId,
        versionId,
      });
      return data?.code === 0;
    },
    onSuccess: (_result, variables) => {
      // Invalidate both version-list caches so the deleted version disappears
      // without a manual refresh: the version dialog reads
      // AgentKeys.versionList (canvas id from route params) and the document
      // pipeline dialog reads the canvasVersionList key.
      queryClient.invalidateQueries({
        queryKey: AgentKeys.versionList(variables.canvasId),
      });
      queryClient.invalidateQueries({
        queryKey: ['canvasVersionList', variables.canvasId],
      });
    },
  });

  return { loading, isError, deleteVersion: mutateAsync };
};

export const useApplyVersion = () => {
  const queryClient = useQueryClient();
  const { isPending: loading, mutateAsync } = useMutation({
    mutationFn: async ({
      canvasId,
      versionId,
    }: {
      canvasId: string;
      versionId: string;
    }) => {
      const { data } = await agentService.applyVersion({
        agentId: canvasId,
        versionId,
      });
      return data?.code === 0;
    },
    onSuccess: (ok, { canvasId }) => {
      if (!ok) return;
      queryClient.invalidateQueries({ queryKey: AgentKeys.list() });
      queryClient.invalidateQueries({
        queryKey: AgentKeys.versionList(canvasId),
      });
      queryClient.invalidateQueries({ queryKey: AgentKeys.detail(canvasId) });
    },
  });

  return { loading, applyVersion: mutateAsync };
};

export const useUpdateVersion = () => {
  const queryClient = useQueryClient();
  const {
    isPending: loading,
    isError,
    mutateAsync,
  } = useMutation({
    mutationFn: async ({
      canvasId,
      versionId,
      alias,
    }: {
      canvasId: string;
      versionId: string;
      alias: string;
    }) => {
      const { data } = await updateVersion(canvasId, versionId, { alias });
      return data?.code === 0;
    },
    onSuccess: (_result, variables) => {
      // The alias edit does not create a version or change the DSL, but the
      // version-list cache must reflect the new alias without a manual refresh
      // (design G2/§4.4).
      queryClient.invalidateQueries({
        queryKey: AgentKeys.versionList(variables.canvasId),
      });
      queryClient.invalidateQueries({
        queryKey: ['canvasVersionList', variables.canvasId],
      });
    },
  });

  return { loading, isError, updateVersion: mutateAsync };
};

export const useFetchVersion = (
  version_id?: string,
): {
  data?: IFlow;
  loading: boolean;
  isError: boolean;
} => {
  const { id } = useParams();
  const {
    data,
    isFetching: loading,
    isError,
  } = useQuery({
    queryKey: AgentKeys.version(id, version_id),
    initialData: undefined,
    gcTime: 0,
    // A pruned version never recovers on retry, and each attempt would
    // re-fire the global error notification.
    retry: false,
    enabled: !!id && !!version_id,
    queryFn: async () => {
      if (!id || !version_id) return undefined;

      // The dialog renders its own error state, so suppress the global
      // notification whose "no permission" wording misleads here.
      const { data } = await agentService.fetchVersion(
        {
          agentId: id,
          versionId: version_id,
          skipGlobalErrorNotification: true,
        },
        true,
      );

      if (data?.code !== 0) {
        throw new Error(data?.message);
      }

      return data?.data ?? undefined;
    },
  });

  return { data, loading, isError };
};

export const useFetchAgentLog = (searchParams: IAgentLogsRequest) => {
  const { id } = useParams();
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IAgentLogsResponse>({
    queryKey: [AgentApiAction.FetchAgentLog, id, searchParams],
    initialData: {} as IAgentLogsResponse,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await fetchAgentLogsByCanvasId(id as string, {
        ...searchParams,
      });

      return { total: data?.total ?? 0, sessions: data?.data ?? [] };
    },
  });

  return { data, loading, refetch };
};

export const AgentSessionKeys = {
  all: [AgentApiAction.FetchSessionsByCanvasId] as const,
  list: (canvasId?: string, tenantId?: string) =>
    [...AgentSessionKeys.all, canvasId, tenantId] as const,
};

export const useFetchSessionsByCanvasId = () => {
  const { id: canvasId } = useParams();
  const { data: tenantInfo } = useFetchTenantInfo();

  return useSessionList<IAgentLogResponse>({
    queryKey: AgentSessionKeys.list(canvasId, tenantInfo.tenant_id),
    enabled: !!canvasId && !isEmpty(tenantInfo),
    fetchPage: async (params) => {
      const { data } = await fetchAgentLogsByCanvasId(canvasId!, {
        ...params,
        exp_user_id: tenantInfo.tenant_id,
      });

      return { total: data?.total ?? 0, items: data?.data ?? [] };
    },
  });
};

export const useFetchExternalAgentInputs = () => {
  const { sharedId } = useGetSharedChatSearchParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IInputs>({
    queryKey: [AgentApiAction.FetchExternalAgentInputs, sharedId],
    initialData: {} as IInputs,
    refetchOnReconnect: false,
    // refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    enabled: !!sharedId,
    queryFn: async () => {
      const { data } = await agentService.fetchExternalAgentInputs(sharedId!);

      return data?.data ?? {};
    },
  });

  return { data, loading, refetch };
};

export const useFetchPrompt = () => {
  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<Record<string, string>>({
    queryKey: [AgentApiAction.FetchPrompt],
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.fetchPrompt();

      return data?.data ?? {};
    },
  });

  return { data, loading, refetch };
};

export const useFetchAgentList = ({
  canvas_category,
}: IPipeLineListRequest) => {
  const { data, isFetching: loading } = useQuery<{
    canvas: AgentListItem[];
    total: number;
  }>({
    queryKey: AgentKeys.listAll(canvas_category),
    initialData: { canvas: [], total: 0 },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await fetchPipeLineList({
        canvas_category,
      });

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export interface IAgentOwnerFilter {
  id: string;
  label: string;
  count: number;
}

export interface IAgentCategoryFilter {
  id: string;
  count: number;
}

export const useFetchAgentFilters = (canvasCategory?: string) => {
  const { data, isFetching: loading } = useQuery<{
    filter: {
      owner: IAgentOwnerFilter[];
      canvas_category: IAgentCategoryFilter[];
    };
    total: number;
  }>({
    queryKey: AgentKeys.filters(canvasCategory),
    initialData: { filter: { owner: [], canvas_category: [] }, total: 0 },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.listAgents(
        {
          params: {
            type: 'filter',
            ...(canvasCategory ? { canvas_category: canvasCategory } : {}),
          },
        },
        true,
      );
      return (
        data?.data ?? { filter: { owner: [], canvas_category: [] }, total: 0 }
      );
    },
  });

  return {
    data: data?.filter ?? { owner: [], canvas_category: [] },
    loading,
  };
};

export const BuiltinPipelineKeys = {
  list: (type: string) =>
    [AgentApiAction.FetchBuiltinPipelineList, type] as const,
  detail: (id: string) =>
    [AgentApiAction.FetchBuiltinPipelineDetail, id] as const,
};

export const useFetchBuiltinPipelines = () => {
  const { data, isFetching: loading } = useQuery<IBuiltinPipelineListResponse>({
    queryKey: BuiltinPipelineKeys.list('builtin'),
    initialData: { canvas: [], total: 0 },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.listBuiltinPipelines(
        { params: { type: 'builtin' } },
        true,
      );
      return data?.data ?? { canvas: [], total: 0 };
    },
  });

  const options =
    data?.canvas?.map((item) => ({
      label: item.title,
      value: item.id,
    })) ?? [];

  return { data, loading, options };
};

export const useCancelDataflow = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.CancelDataflow],
    mutationFn: async (taskId: string) => {
      const ret = await agentService.cancelDataflow(taskId);
      if (ret?.data?.code === 0) {
        message.success('success');
      } else {
        message.error(ret?.data?.data);
      }
      return ret?.data?.code;
    },
  });

  return { data, loading, cancelDataflow: mutateAsync };
};

// export const useFetchKnowledgeList = () => {
//   const { data, isFetching: loading } = useQuery<IFlow[]>({
//     queryKey: [AgentApiAction.FetchAgentList],
//     initialData: [],
//     gcTime: 0, // https://tanstack.com/query/latest/docs/framework/react/guides/caching?from=reactQueryV3
//     queryFn: async () => {
//       const { data } = await agentService.listAgents();

//       return data?.data ?? [];
//     },
//   });

//   return { list: data, loading };
// };

export function useCancelConversation() {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.CancelCanvas],
    mutationFn: async (taskId: string) => {
      const ret = await agentService.cancelCanvas(taskId);

      return ret?.data?.code;
    },
  });

  return { data, loading, cancelConversation: mutateAsync };
}

export const useFetchFlowSSE = (): {
  data: IFlow;
  loading: boolean;
  refetch: () => void;
} => {
  const { sharedId } = useGetSharedChatSearchParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery({
    queryKey: [AgentApiAction.FetchSharedAgent, sharedId],
    initialData: {} as IFlow,
    enabled: !!sharedId,
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: async () => {
      if (!sharedId) return {};
      const { data } = await agentService.getAgent(sharedId);

      const messageList = buildMessageListWithUuid(
        get(data, 'data.dsl.messages', []),
      );
      set(data, 'data.dsl.messages', messageList);

      return data?.data ?? {};
    },
  });

  return { data, loading, refetch };
};

export const useFetchWebhookTrace = (autoStart: boolean = true) => {
  const { id } = useParams();
  const [currentWebhookId, setCurrentWebhookId] = useState<string>('');
  const [currentNextSinceTs, setCurrentNextSinceTs] = useState<number>(0);
  const [shouldPoll, setShouldPoll] = useState(autoStart);

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IWebhookTrace>({
    queryKey: [AgentApiAction.FetchWebhookTrace, id],
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchIntervalInBackground: false,
    gcTime: 0,
    enabled: !!id && shouldPoll,
    queryFn: async () => {
      if (!id) return {};

      const payload: IAgentWebhookTraceRequest =
        {} as IAgentWebhookTraceRequest;

      if (currentNextSinceTs) {
        payload['since_ts'] = currentNextSinceTs;
      }

      if (currentWebhookId) {
        payload['webhook_id'] = currentWebhookId;
      }

      const { data } = await fetchWebhookTrace(id, payload);

      const result = data.data ?? {};

      if (result.webhook_id && result.webhook_id !== currentWebhookId) {
        setCurrentWebhookId(result.webhook_id);
      }

      if (
        currentNextSinceTs === 0 &&
        result.next_since_ts &&
        result.next_since_ts !== currentNextSinceTs
      ) {
        setCurrentNextSinceTs(result.next_since_ts);
      }

      if (result.finished) {
        setShouldPoll(false);
      }

      return result;
    },
    refetchInterval: shouldPoll ? 3000 : false,
  });

  return {
    data,
    loading,
    refetch,
    isPolling: shouldPoll,
    currentWebhookId,
    currentNextSinceTs,
  };
};

export function useCreateAgentWebhookCredential() {
  const { id } = useParams();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [AgentApiAction.CreateWebhookCredential],
    mutationFn: async () => {
      if (!id) throw new Error('Agent ID is required');

      const { data } = await issueAgentWebhookCredential(id);
      if (data?.code !== 0 || !data?.data?.webhook_url) {
        throw new Error(data?.message || 'Failed to create webhook URL');
      }

      return data.data as IAgentWebhookCredential;
    },
  });

  return { createWebhookCredential: mutateAsync, loading };
}

export function useRevokeAgentWebhookCredential() {
  const { id } = useParams();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [AgentApiAction.RevokeWebhookCredential],
    mutationFn: async () => {
      if (!id) throw new Error('Agent ID is required');

      const { data } = await revokeAgentWebhookCredential(id);
      if (data?.code !== 0) {
        throw new Error(data?.message || 'Failed to revoke webhook URL');
      }
    },
  });

  return { revokeWebhookCredential: mutateAsync, loading };
}

export function useCreateAgentSession() {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.CreateAgentSession],
    mutationFn: async (payload: { id: string; name: string }) => {
      const { data } = await createAgentSession(payload);

      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: AgentSessionKeys.all,
        });
        return data.data;
      }

      throw new Error(data.message);
    },
  });

  return { data, loading, createAgentSession: mutateAsync };
}

export function useDeleteAgentSession() {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.DeleteAgentSession],
    mutationFn: async ({
      canvasId,
      sessionId,
    }: {
      canvasId: string;
      sessionId: string;
    }) => {
      const { data } = await deleteAgentSession(canvasId, sessionId);

      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: AgentSessionKeys.all,
        });
      }

      return data?.code ?? -1;
    },
  });

  return { data, loading, deleteAgentSession: mutateAsync };
}

export function useFetchSessionManually() {
  const { id: canvasId } = useParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<IAgentLogResponse, unknown, string>({
    mutationKey: [AgentApiAction.FetchSessionByIdManually, canvasId],
    mutationFn: async (sessionId) => {
      if (!canvasId || !sessionId) {
        return null;
      }

      const { data } = await fetchAgentLogsById(canvasId, sessionId);

      return data?.data;
    },
  });

  return { data, loading, fetchSessionManually: mutateAsync };
}

export const useExportAgentLog = () => {
  const { id } = useParams();
  const { mutateAsync, isPending: loading } = useMutation({
    mutationKey: [AgentApiAction.FetchAgentLog, 'export', id],
    mutationFn: async (searchParams: IAgentLogsRequest) => {
      const { data } = await fetchAgentLogsByCanvasId(id as string, {
        ...searchParams,
      });

      return data?.data ?? [];
    },
  });

  return { exportLogs: mutateAsync, loading };
};

export const useFetchPipelineDslByPipelineId = (
  pipelineId?: string,
  isBuiltin = false,
) => {
  const { data: dsl, isFetching: loading } = useQuery({
    queryKey: isBuiltin
      ? BuiltinPipelineKeys.detail(pipelineId!)
      : [AgentApiAction.FetchPipelineNodes, pipelineId],
    initialData: {},
    gcTime: 0,
    enabled: !!pipelineId,
    queryFn: async () => {
      const { data } = isBuiltin
        ? await agentService.getBuiltinPipeline(pipelineId!)
        : await agentService.getAgent(pipelineId!);
      const flow = data?.data;
      return flow?.dsl ?? {};
    },
  });

  return { dsl, loading };
};
