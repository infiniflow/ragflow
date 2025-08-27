import { FileUploadProps } from '@/components/file-upload';
import message from '@/components/ui/message';
import { AgentGlobals } from '@/constants/agent';
import {
  DSL,
  IAgentLogsRequest,
  IAgentLogsResponse,
  IFlow,
  IFlowTemplate,
  ITraceData,
} from '@/interfaces/database/agent';
import { IDebugSingleRequestBody } from '@/interfaces/request/agent';
import i18n from '@/locales/config';
import { BeginId } from '@/pages/agent/constant';
import { IInputs } from '@/pages/agent/interface';
import { useGetSharedChatSearchParams } from '@/pages/chat/shared-hooks';
import agentService, {
  fetchAgentLogsByCanvasId,
  fetchTrace,
} from '@/services/agent-service';
import api from '@/utils/api';
import { buildMessageListWithUuid } from '@/utils/chat';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { get, set } from 'lodash';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useSearchParams } from 'umi';
import { v4 as uuid } from 'uuid';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';

export const enum AgentApiAction {
  FetchAgentList = 'fetchAgentList',
  UpdateAgentSetting = 'updateAgentSetting',
  DeleteAgent = 'deleteAgent',
  FetchAgentDetail = 'fetchAgentDetail',
  ResetAgent = 'resetAgent',
  SetAgent = 'setAgent',
  FetchAgentTemplates = 'fetchAgentTemplates',
  UploadCanvasFile = 'uploadCanvasFile',
  UploadCanvasFileWithProgress = 'uploadCanvasFileWithProgress',
  Trace = 'trace',
  TestDbConnect = 'testDbConnect',
  DebugSingle = 'debugSingle',
  FetchInputForm = 'fetchInputForm',
  FetchVersionList = 'fetchVersionList',
  FetchVersion = 'fetchVersion',
  FetchAgentAvatar = 'fetchAgentAvatar',
  FetchExternalAgentInputs = 'fetchExternalAgentInputs',
  SetAgentSetting = 'setAgentSetting',
}

export const EmptyDsl = {
  graph: {
    nodes: [
      {
        id: BeginId,
        type: 'beginNode',
        position: {
          x: 50,
          y: 200,
        },
        data: {
          label: 'Begin',
          name: 'begin',
        },
        sourcePosition: 'left',
        targetPosition: 'right',
      },
    ],
    edges: [],
  },
  components: {
    begin: {
      obj: {
        component_name: 'Begin',
        params: {},
      },
      downstream: ['Answer:China'], // other edge target is downstream, edge source is current node id
      upstream: [], // edge source is upstream, edge target is current node id
    },
  },
  retrieval: [], // reference
  history: [],
  path: [],
  globals: {
    [AgentGlobals.SysQuery]: '',
    [AgentGlobals.SysUserId]: '',
    [AgentGlobals.SysConversationTurns]: 0,
    [AgentGlobals.SysFiles]: [],
  },
};

export const useFetchAgentTemplates = () => {
  const { t } = useTranslation();

  const { data } = useQuery<IFlowTemplate[]>({
    queryKey: [AgentApiAction.FetchAgentTemplates],
    initialData: [],
    queryFn: async () => {
      const { data } = await agentService.listTemplates();
      if (Array.isArray(data?.data)) {
        data.data.unshift({
          id: uuid(),
          title: t('flow.blank'),
          description: t('flow.createFromNothing'),
          dsl: EmptyDsl,
        });
      }

      return data.data;
    },
  });

  return data;
};

export const useFetchAgentListByPage = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const { data, isFetching: loading } = useQuery<{
    canvas: IFlow[];
    total: number;
  }>({
    queryKey: [
      AgentApiAction.FetchAgentList,
      {
        debouncedSearchString,
        ...pagination,
      },
    ],
    initialData: { canvas: [], total: 0 },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.listCanvasTeam(
        {
          params: {
            keywords: debouncedSearchString,
            page_size: pagination.pageSize,
            page: pagination.current,
          },
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
    data: data.canvas,
    loading,
    searchString,
    handleInputChange: onInputChange,
    pagination: { ...pagination, total: data?.total },
    setPagination,
  };
};

export const useUpdateAgentSetting = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.UpdateAgentSetting],
    mutationFn: async (params: any) => {
      const ret = await agentService.settingCanvas(params);
      if (ret?.data?.code === 0) {
        message.success('success');
        queryClient.invalidateQueries({
          queryKey: [AgentApiAction.FetchAgentList],
        });
      } else {
        message.error(ret?.data?.data);
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
    mutationFn: async (canvasIds: string[]) => {
      const { data } = await agentService.removeCanvas({ canvasIds });
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [AgentApiAction.FetchAgentList],
        });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteAgent: mutateAsync };
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
    queryKey: [AgentApiAction.FetchAgentDetail],
    initialData: {} as IFlow,
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.fetchCanvas(sharedId || id);

      const messageList = buildMessageListWithUuid(
        get(data, 'data.dsl.messages', []),
      );
      set(data, 'data.dsl.messages', messageList);

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
      const { data } = await agentService.resetCanvas({ id });
      return data;
    },
  });

  return { data, loading, resetAgent: mutateAsync };
};

export const useSetAgent = (showMessage: boolean = true) => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.SetAgent],
    mutationFn: async (params: {
      id?: string;
      title?: string;
      dsl?: DSL;
      avatar?: string;
    }) => {
      const { data = {} } = await agentService.setCanvas(params);
      if (data.code === 0) {
        if (showMessage) {
          message.success(
            i18n.t(`message.${params?.id ? 'modified' : 'created'}`),
          );
        }
        queryClient.invalidateQueries({
          queryKey: [AgentApiAction.FetchAgentList],
        });
      }
      return data;
    },
  });

  return { data, loading, setAgent: mutateAsync };
};

// Only one file can be uploaded at a time
export const useUploadCanvasFile = () => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const shared_id = searchParams.get('shared_id');
  const canvasId = id || shared_id;
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.UploadCanvasFile],
    mutationFn: async (body: any) => {
      let nextBody = body;
      try {
        if (Array.isArray(body)) {
          nextBody = new FormData();
          body.forEach((file: File) => {
            nextBody.append('file', file as any);
          });
        }

        const { data } = await agentService.uploadCanvasFile(
          { url: api.uploadAgentFile(canvasId as string), data: nextBody },
          true,
        );
        if (data?.code === 0) {
          message.success(i18n.t('message.uploaded'));
        }
        return data;
      } catch (error) {
        message.error('error');
      }
    },
  });

  return { data, loading, uploadCanvasFile: mutateAsync };
};

export const useUploadCanvasFileWithProgress = (
  identifier?: Nullable<string>,
) => {
  const { id } = useParams();

  type UploadParameters = Parameters<NonNullable<FileUploadProps['onUpload']>>;

  type X = { files: UploadParameters[0]; options: UploadParameters[1] };

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.UploadCanvasFileWithProgress],
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

        const { data } = await agentService.uploadCanvasFile(
          {
            url: api.uploadAgentFile(identifier || id),
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
        message.error(error?.message);
      }
    },
  });

  return { data, loading, uploadCanvasFile: mutateAsync };
};

export const useFetchMessageTrace = (
  isStopFetchTrace: boolean,
  canvasId?: string,
) => {
  const { id } = useParams();
  const queryId = id || canvasId;
  const [messageId, setMessageId] = useState('');

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<ITraceData[]>({
    queryKey: [AgentApiAction.Trace, queryId, messageId],
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    enabled: !!queryId && !!messageId,
    refetchInterval: !isStopFetchTrace ? 3000 : false,
    queryFn: async () => {
      const { data } = await fetchTrace({
        canvas_id: queryId as string,
        message_id: messageId,
      });

      return data?.data ?? [];
    },
  });

  return { data, loading, refetch, setMessageId };
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
        message.success(ret?.data?.data);
      } else {
        message.error(ret?.data?.data);
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
    mutationKey: [AgentApiAction.FetchInputForm],
    mutationFn: async (params: IDebugSingleRequestBody) => {
      const ret = await agentService.debugSingle({ id, ...params });
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
    queryKey: [AgentApiAction.FetchInputForm],
    initialData: {},
    enabled: !!id && !!componentId,
    queryFn: async () => {
      const { data } = await agentService.inputForm(
        {
          params: {
            id,
            component_id: componentId,
          },
        },
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
    Array<{ created_at: string; title: string; id: string }>
  >({
    queryKey: [AgentApiAction.FetchVersionList],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await agentService.fetchVersionList(id);

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useFetchVersion = (
  version_id?: string,
): {
  data?: IFlow;
  loading: boolean;
} => {
  const { data, isFetching: loading } = useQuery({
    queryKey: [AgentApiAction.FetchVersion, version_id],
    initialData: undefined,
    gcTime: 0,
    enabled: !!version_id, // Only call API when both values are provided
    queryFn: async () => {
      if (!version_id) return undefined;

      const { data } = await agentService.fetchVersion(version_id);

      return data?.data ?? undefined;
    },
  });

  return { data, loading };
};

export const useFetchAgentAvatar = (): {
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
    queryKey: [AgentApiAction.FetchAgentAvatar],
    initialData: {} as IFlow,
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: async () => {
      if (!sharedId) return {};
      const { data } = await agentService.fetchAgentAvatar(sharedId);

      return data?.data ?? {};
    },
  });

  return { data, loading, refetch };
};

export const useFetchAgentLog = (searchParams: IAgentLogsRequest) => {
  const { id } = useParams();
  const { data, isFetching: loading } = useQuery<IAgentLogsResponse>({
    queryKey: ['fetchAgentLog', id, searchParams],
    initialData: {} as IAgentLogsResponse,
    gcTime: 0,
    queryFn: async () => {
      console.log('useFetchAgentLog', searchParams);
      const { data } = await fetchAgentLogsByCanvasId(id as string, {
        ...searchParams,
      });

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useFetchExternalAgentInputs = () => {
  const { sharedId } = useGetSharedChatSearchParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IInputs>({
    queryKey: [AgentApiAction.FetchExternalAgentInputs],
    initialData: {} as IInputs,
    refetchOnReconnect: false,
    refetchOnMount: false,
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

export const useSetAgentSetting = () => {
  const { id } = useParams();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [AgentApiAction.SetAgentSetting],
    mutationFn: async (params: any) => {
      const ret = await agentService.settingCanvas({ id, ...params });
      if (ret?.data?.code === 0) {
        message.success('success');
        queryClient.invalidateQueries({
          queryKey: [AgentApiAction.FetchAgentDetail],
        });
      } else {
        message.error(ret?.data?.data);
      }
      return ret?.data?.code;
    },
  });

  return { data, loading, setAgentSetting: mutateAsync };
};
