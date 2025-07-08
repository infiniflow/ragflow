import { AgentGlobals } from '@/constants/agent';
import { ITraceData } from '@/interfaces/database/agent';
import { DSL, IFlow, IFlowTemplate } from '@/interfaces/database/flow';
import i18n from '@/locales/config';
import { BeginId } from '@/pages/agent/constant';
import { useGetSharedChatSearchParams } from '@/pages/chat/shared-hooks';
import flowService from '@/services/flow-service';
import { buildMessageListWithUuid } from '@/utils/chat';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { message } from 'antd';
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
      const { data } = await flowService.listTemplates();
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
      const { data } = await flowService.listCanvasTeam({
        keywords: debouncedSearchString,
        page_size: pagination.pageSize,
        page: pagination.current,
      });

      return data?.data ?? [];
    },
  });

  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      // setPagination({ page: 1 }); // TODO: 这里导致重复请求
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
      const ret = await flowService.settingCanvas(params);
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
      const { data } = await flowService.removeCanvas({ canvasIds });
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
      const { data } = await flowService.getCanvas({}, sharedId || id);

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
      const { data } = await flowService.resetCanvas({ id });
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
      const { data = {} } = await flowService.setCanvas(params);
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

        const { data } = await flowService.uploadCanvasFile(nextBody);
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
      const { data } = await flowService.trace({
        canvas_id: id,
        message_id: messageId,
      });

      return data?.data ?? [];
    },
  });

  return { data, loading, refetch, setMessageId };
};
