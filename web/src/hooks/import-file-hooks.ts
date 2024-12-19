import { useGetPagination } from '@/hooks/logic-hooks';
import { ResponseType } from '@/interfaces/database/base';
import fileManagerService from '@/services/file-manager-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { PaginationProps } from 'antd';

export interface IImportListResult {
  name: string;
  size: string;
  etag: string;
  owner: string;
}

export interface IListResult {
  data: IImportListResult[];
  pagination: PaginationProps;
  setPagination: (pagination: { page: number; pageSize: number }) => void;
  loading: boolean;
}

export const useFetchImportFileList = (): ResponseType<any> & IListResult => {
  const { pagination } = useGetPagination();
  const { data, isFetching: loading } = useQuery({
    queryKey: ['getImportFiles', {}],
    initialData: {},
    gcTime: 0,
    queryFn: async (params: any) => {
      console.info(params);
      const { data } = await fileManagerService.getImportFiles();
      return data;
    },
  });

  return {
    ...data,
    pagination: { ...pagination, total: data?.data?.total },
    loading,
  };
};

export const useImportFiles = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['importFiles'],
    mutationFn: async (params: { keys: string[]; dir: boolean }) => {
      const { data = {} } = await fileManagerService.importFiles(params);
      return data;
    },
  });

  return { data, loading, importFiles: mutateAsync };
};
