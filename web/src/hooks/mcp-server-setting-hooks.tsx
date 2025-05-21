import { ResponseGetType } from '@/interfaces/database/base';
import { IMcpServerInfo } from '@/interfaces/database/mcp-server';
import mcpServerService, { getMcpServer } from '@/services/mcp-server-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { message } from 'antd';
import { useTranslation } from 'react-i18next';

export const useFetchMcpServerList = (): ResponseGetType<IMcpServerInfo[]> => {
  const { data, isFetching: loading } = useQuery({
    queryKey: ['mcpServerList'],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await mcpServerService.get_list();
      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useFetchMcpServerInfo = (id?: string): ResponseGetType<IMcpServerInfo | null> => {
  if (!id) {
    return { data: null, loading: false };
  }

  const { data, isFetching: loading } = useQuery({
    queryKey: ['mcpServerInfo'],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data } = await getMcpServer(id);
      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useCreateMcpServer = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['createMcpServer'],
    mutationFn: async (serverInfo: IMcpServerInfo) => {
      const { data } = await mcpServerService.add(serverInfo);
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: ['mcpServerList'] });
      }
      return data?.code;
    },
  });

  return { data, loading, createMcpServer: mutateAsync };
};

export const useUpdateMcpServer = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['updateMcpServer'],
    mutationFn: async (serverInfo: IMcpServerInfo) => {
      const { data } = await mcpServerService.update(serverInfo);
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: ['mcpServerList'] });
      }
      return data?.code;
    },
  });

  return { data, loading, updateMcpServer: mutateAsync };
};

export const useDeleteMcpServer = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteMcpServer'],
    mutationFn: async ({
      id
    }: {
      id: string;
    }) => {
      const { data } = await mcpServerService.rm({
        id
      });
      if (data.code === 0) {
        message.success(t('message.deleted'));
        queryClient.invalidateQueries({ queryKey: ['mcpServerList'] });
        queryClient.invalidateQueries({ queryKey: ['mcpServerInfo'] });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteMcpServer: mutateAsync };
};
