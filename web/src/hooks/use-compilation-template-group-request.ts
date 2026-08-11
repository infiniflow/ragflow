import message from '@/components/ui/message';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import {
  ICreateCompilationTemplateGroupRequestBody,
  IUpdateCompilationTemplateGroupRequestBody,
} from '@/interfaces/request/compilation-template';
import i18n from '@/locales/config';
import {
  compilationTemplateGroupService,
  createCompilationTemplateGroup,
  deleteCompilationTemplateGroup,
  getCompilationTemplateGroup,
  updateCompilationTemplateGroup,
} from '@/services/compilation-template-group-service';
import { isCreateCompilationTemplateGroup } from '@/utils/compilation-template-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { useCallback, useMemo } from 'react';
import { useParams } from 'react-router';

import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';

export const enum CompilationTemplateGroupApiAction {
  FetchCompilationTemplateGroups = 'fetchCompilationTemplateGroups',
  FetchCompilationTemplateGroup = 'fetchCompilationTemplateGroup',
  CreateCompilationTemplateGroup = 'createCompilationTemplateGroup',
  UpdateCompilationTemplateGroup = 'updateCompilationTemplateGroup',
  DeleteCompilationTemplateGroup = 'deleteCompilationTemplateGroup',
}

export const CompilationTemplateGroupKeys = {
  list: (keywords?: string, page?: number, pageSize?: number) =>
    [
      CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups,
      { keywords, page, pageSize },
    ] as const,
  detail: (id?: string) =>
    [
      CompilationTemplateGroupApiAction.FetchCompilationTemplateGroup,
      id,
    ] as const,
  all: () =>
    [CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups] as const,
};

export const useFetchCompilationTemplateGroupsByPage = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const { data, isFetching: loading } = useQuery<{
    groups: ICompilationTemplateGroup[];
    total: number;
  }>({
    queryKey: CompilationTemplateGroupKeys.list(
      debouncedSearchString,
      pagination.current,
      pagination.pageSize,
    ),
    initialData: {
      groups: [],
      total: 0,
    },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await compilationTemplateGroupService.listGroups(
        {
          params: {
            keywords: debouncedSearchString,
            page: pagination.current,
            page_size: pagination.pageSize,
          },
        },
        true,
      );

      return {
        groups: (data?.data?.groups ?? []) as ICompilationTemplateGroup[],
        total: data?.data?.total ?? 0,
      };
    },
  });

  const currentPagination = useMemo(
    () => ({ ...pagination, total: data?.total ?? 0 }),
    [pagination, data?.total],
  );

  return {
    groups: data?.groups ?? [],
    total: data?.total ?? 0,
    searchString,
    handleInputChange,
    pagination: currentPagination,
    setPagination,
    loading,
  };
};

export const useFetchCompilationTemplateGroup = () => {
  const { id } = useParams<{ id: string }>();
  const isCreate = isCreateCompilationTemplateGroup(id);

  const { data, isFetching: loading } = useQuery<
    ICompilationTemplateGroup | undefined
  >({
    queryKey: CompilationTemplateGroupKeys.detail(id),
    enabled: !isCreate,
    gcTime: 0,
    queryFn: async () => {
      if (isCreate) return undefined;
      const { data } = await getCompilationTemplateGroup(id);
      return data?.data as ICompilationTemplateGroup | undefined;
    },
  });

  return { data, loading };
};

export const useCreateCompilationTemplateGroup = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [
      CompilationTemplateGroupApiAction.CreateCompilationTemplateGroup,
    ],
    mutationFn: async (params: ICreateCompilationTemplateGroupRequestBody) => {
      const { data } = await createCompilationTemplateGroup(params);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [
          CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups,
        ],
      });
    },
  });

  const createGroup = useCallback(
    async (params: ICreateCompilationTemplateGroupRequestBody) => {
      const result = await mutateAsync(params);
      if (result.code === 0) {
        message.success(i18n.t('message.created'));
      }
      return result;
    },
    [mutateAsync],
  );

  return { data, loading, createGroup };
};

export const useUpdateCompilationTemplateGroup = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [
      CompilationTemplateGroupApiAction.UpdateCompilationTemplateGroup,
    ],
    mutationFn: async ({
      id,
      params,
    }: {
      id: string;
      params: IUpdateCompilationTemplateGroupRequestBody;
    }) => {
      const { data } = await updateCompilationTemplateGroup(id, params);
      return data;
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: [
          CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups,
        ],
      });
      queryClient.invalidateQueries({
        queryKey: CompilationTemplateGroupKeys.detail(variables.id),
      });
    },
  });

  const updateGroup = useCallback(
    async (id: string, params: IUpdateCompilationTemplateGroupRequestBody) => {
      const result = await mutateAsync({ id, params });
      if (result.code === 0) {
        message.success(i18n.t('message.modified'));
      }
      return result;
    },
    [mutateAsync],
  );

  return { data, loading, updateGroup };
};

export const useDeleteCompilationTemplateGroup = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [
      CompilationTemplateGroupApiAction.DeleteCompilationTemplateGroup,
    ],
    mutationFn: async (id: string) => {
      const { data } = await deleteCompilationTemplateGroup(id);
      if (data.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: [
            CompilationTemplateGroupApiAction.FetchCompilationTemplateGroups,
          ],
        });
      }
      return data?.data ?? true;
    },
  });

  const deleteGroup = useCallback(
    async (id: string) => {
      await mutateAsync(id);
    },
    [mutateAsync],
  );

  return { data, loading, deleteGroup };
};

export const useFetchAllCompilationTemplateGroups = () => {
  const { data, isFetching: loading } = useQuery<ICompilationTemplateGroup[]>({
    queryKey: CompilationTemplateGroupKeys.all(),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await compilationTemplateGroupService.listGroups(
        {
          params: { keywords: '', page: 1, page_size: 100 },
        },
        true,
      );
      return (data?.data?.groups ?? []) as ICompilationTemplateGroup[];
    },
  });

  return { groups: data ?? [], loading };
};

export const useCompilationTemplateGroupOptions = () => {
  const { groups } = useFetchAllCompilationTemplateGroups();

  return useMemo(
    () => groups.map((group) => ({ label: group.name, value: group.id })),
    [groups],
  );
};
