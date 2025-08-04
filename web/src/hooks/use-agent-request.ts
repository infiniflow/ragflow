import message from '@/components/ui/message';
import { AgentGlobals } from '@/constants/agent';
import { ITraceData } from '@/interfaces/database/agent';
import { DSL, IFlow, IFlowTemplate } from '@/interfaces/database/flow';
import { IDebugSingleRequestBody } from '@/interfaces/request/agent';
import i18n from '@/locales/config';
import { BeginId } from '@/pages/agent/constant';
import { useGetSharedChatSearchParams } from '@/pages/chat/shared-hooks';
import agentService from '@/services/agent-service';
import { buildMessageListWithUuid } from '@/utils/chat';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { get, set } from 'lodash';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
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
  Trace = 'trace',
  TestDbConnect = 'testDbConnect',
  DebugSingle = 'debugSingle',
  FetchInputForm = 'fetchInputForm',
  FetchVersionList = 'fetchVersionList',
  FetchVersion = 'fetchVersion',
  FetchAgentAvatar = 'fetchAgentAvatar',
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
    kbs: IFlow[];
    total: number;
  }>({
    queryKey: [
      AgentApiAction.FetchAgentList,
      {
        debouncedSearchString,
        ...pagination,
      },
    ],
    initialData: { kbs: [], total: 0 },
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

      return data?.data ?? [];
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
    data: data.kbs,
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

export const useSetAgent = () => {
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
        message.success(
          i18n.t(`message.${params?.id ? 'modified' : 'created'}`),
        );
        queryClient.invalidateQueries({
          queryKey: [AgentApiAction.FetchAgentList],
        });
      }
      return data;
    },
  });

  return { data, loading, setAgent: mutateAsync };
};

export const useUploadCanvasFile = () => {
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

        const { data } = await agentService.uploadCanvasFile(nextBody);
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

export const useFetchMessageTrace = () => {
  const { id } = useParams();
  const [messageId, setMessageId] = useState('');

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<ITraceData[]>({
    queryKey: [AgentApiAction.Trace, id, messageId],
    refetchOnReconnect: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    enabled: !!id && !!messageId,
    refetchInterval: 3000,
    queryFn: async () => {
      const { data } = await agentService.trace({
        canvas_id: id,
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
      const { data } = await agentService.inputForm({
        id,
        component_id: componentId,
      });

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
