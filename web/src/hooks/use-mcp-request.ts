import message from '@/components/ui/message';
import { ResponseType } from '@/interfaces/database/base';
import {
  IExportedMcpServers,
  IMcpServer,
  IMcpServerListResponse,
  IMCPTool,
  IMCPToolRecord,
} from '@/interfaces/database/mcp';
import {
  IImportMcpServersRequestBody,
  ITestMcpRequestBody,
} from '@/interfaces/request/mcp';
import i18n from '@/locales/config';
import mcpServerService, {
  listMcpServers,
} from '@/services/mcp-server-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { useState } from 'react';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';

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
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const { data, isFetching: loading } = useQuery<IMcpServerListResponse>({
    queryKey: [
      McpApiAction.ListMcpServer,
      {
        debouncedSearchString,
        ...pagination,
      },
    ],
    initialData: { total: 0, mcp_servers: [] },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listMcpServers({
        keywords: debouncedSearchString,
        page_size: pagination.pageSize,
        page: pagination.current,
      });
      return data?.data;
    },
  });

  return {
    data,
    loading,
    handleInputChange,
    setPagination,
    searchString,
    pagination: { ...pagination, total: data?.total },
  };
};

export const useGetMcpServer = (id: string) => {
  const { data, isFetching: loading } = useQuery<IMcpServer>({
    queryKey: [McpApiAction.GetMcpServer, id],
    initialData: {} as IMcpServer,
    gcTime: 0,
    enabled: !!id,
    queryFn: async () => {
      const { data } = await mcpServerService.get({ mcp_id: id });
      return data?.data ?? {};
    },
  });

  return { data, loading, id };
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
    mutationFn: async (params: IImportMcpServersRequestBody) => {
      const { data = {} } = await mcpServerService.import(params);
      if (data.code === 0) {
        message.success(i18n.t(`message.operated`));

        queryClient.invalidateQueries({
          queryKey: [McpApiAction.ListMcpServer],
        });
      }
      return data;
    },
  });

  return { data, loading, importMcpServer: mutateAsync };
};

export const useExportMcpServer = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<ResponseType<IExportedMcpServers>, Error, string[]>({
    mutationKey: [McpApiAction.ExportMcpServer],
    mutationFn: async (ids) => {
      const { data = {} } = await mcpServerService.export({ mcp_ids: ids });
      if (data.code === 0) {
        message.success(i18n.t(`message.operated`));
      }
      return data;
    },
  });

  return { data, loading, exportMcpServer: mutateAsync };
};

export const useListMcpServerTools = () => {
  const [ids, setIds] = useState<string[]>([]);
  const { data, isFetching: loading } = useQuery<IMCPToolRecord>({
    queryKey: [McpApiAction.ListMcpServerTools],
    initialData: {} as IMCPToolRecord,
    gcTime: 0,
    enabled: ids.length > 0,
    queryFn: async () => {
      const { data } = await mcpServerService.listTools({ mcp_ids: ids });
      return data?.data ?? {};
    },
  });

  return { data, loading, setIds };
};

export const useTestMcpServer = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<ResponseType<IMCPTool[]>, Error, ITestMcpRequestBody>({
    mutationKey: [McpApiAction.TestMcpServer],
    mutationFn: async (params) => {
      const { data } = await mcpServerService.test(params);

      return data;
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
