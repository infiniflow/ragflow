import message from '@/components/ui/message';
import { IMcpServerListResponse, IMCPTool } from '@/interfaces/database/mcp';
import { ITestMcpRequestBody } from '@/interfaces/request/mcp';
import i18n from '@/locales/config';
import mcpServerService from '@/services/mcp-server-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

export const enum McpApiAction {
  ListMcpServer = 'listMcpServer',
  GetMcpServer = 'getMcpServer',
  CreateMcpServer = 'createMcpServer',
  UpdateMcpServer = 'updateMcpServer',
  DeleteMcpServer = 'deleteMcpServer',
  ImportMcpServer = 'importMcpServer',
  ExportMcpServer = 'exportMcpServer',
  ListMcpServerTools = 'listMcpServerTools',
  TestMcpServerTool = 'testMcpServerTool',
  CacheMcpServerTool = 'cacheMcpServerTool',
  TestMcpServer = 'testMcpServer',
}

export const useListMcpServer = () => {
  const { data, isFetching: loading } = useQuery<IMcpServerListResponse>({
    queryKey: [McpApiAction.ListMcpServer],
    initialData: { total: 0, mcp_servers: [] },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await mcpServerService.list({});
      return data?.data;
    },
  });

  return { data, loading };
};

export const useGetMcpServer = () => {
  const [id, setId] = useState('');
  const { data, isFetching: loading } = useQuery({
    queryKey: [McpApiAction.GetMcpServer, id],
    initialData: {},
    gcTime: 0,
    enabled: !!id,
    queryFn: async () => {
      const { data } = await mcpServerService.get();
      return data?.data ?? {};
    },
  });

  return { data, loading, setId, id };
};

export const useCreateMcpServer = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [McpApiAction.CreateMcpServer],
    mutationFn: async (params: Record<string, any>) => {
      const { data = {} } = await mcpServerService.create(params);
      if (data.code === 0) {
        message.success(i18n.t(`message.created`));

        queryClient.invalidateQueries({
          queryKey: [McpApiAction.ListMcpServer],
        });
      }
      return data.code;
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
    mutationKey: [McpApiAction.UpdateMcpServer],
    mutationFn: async (params: Record<string, any>) => {
      const { data = {} } = await mcpServerService.update(params);
      if (data.code === 0) {
        message.success(i18n.t(`message.updated`));

        queryClient.invalidateQueries({
          queryKey: [McpApiAction.ListMcpServer],
        });
      }
      return data.code;
    },
  });

  return { data, loading, updateMcpServer: mutateAsync };
};

export const useDeleteMcpServer = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [McpApiAction.DeleteMcpServer],
    mutationFn: async (ids: string[]) => {
      const { data = {} } = await mcpServerService.delete({ mcp_ids: ids });
      if (data.code === 0) {
        message.success(i18n.t(`message.deleted`));

        queryClient.invalidateQueries({
          queryKey: [McpApiAction.ListMcpServer],
        });
      }
      return data;
    },
  });

  return { data, loading, deleteMcpServer: mutateAsync };
};

export const useImportMcpServer = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [McpApiAction.ImportMcpServer],
    mutationFn: async (params: Record<string, any>) => {
      const { data = {} } = await mcpServerService.import(params);
      if (data.code === 0) {
        message.success(i18n.t(`message.created`));

        queryClient.invalidateQueries({
          queryKey: [McpApiAction.ListMcpServer],
        });
      }
      return data;
    },
  });

  return { data, loading, importMcpServer: mutateAsync };
};

export const useListMcpServerTools = () => {
  const { data, isFetching: loading } = useQuery({
    queryKey: [McpApiAction.ListMcpServerTools],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await mcpServerService.listTools();
      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useTestMcpServer = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<IMCPTool[], Error, ITestMcpRequestBody>({
    mutationKey: [McpApiAction.TestMcpServer],
    mutationFn: async (params) => {
      const { data } = await mcpServerService.test(params);

      return data?.data || [];
    },
  });

  return { data, loading, testMcpServer: mutateAsync };
};

export const useCacheMcpServerTool = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [McpApiAction.CacheMcpServerTool],
    mutationFn: async (params: Record<string, any>) => {
      const { data = {} } = await mcpServerService.cacheTool(params);

      return data;
    },
  });

  return { data, loading, cacheMcpServerTool: mutateAsync };
};

export const useTestMcpServerTool = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [McpApiAction.TestMcpServerTool],
    mutationFn: async (params: Record<string, any>) => {
      const { data = {} } = await mcpServerService.testTool(params);

      return data;
    },
  });

  return { data, loading, testMcpServerTool: mutateAsync };
};
