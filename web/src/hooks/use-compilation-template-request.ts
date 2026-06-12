import message from '@/components/ui/message';
import {
  BuiltinCompilationTemplate,
  CompilationTemplate,
  CompilationTemplateListResponse,
} from '@/interfaces/database/compilation-template';
import {
  ICreateCompilationTemplateRequest,
  IUpdateCompilationTemplateRequest,
} from '@/interfaces/request/compilation-template';
import i18n from '@/locales/config';
import compilationTemplateService from '@/services/compilation-template-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';

/**
 * Query-key factory for the knowledge-compilation templates surface.
 * Every `useQuery` and `invalidateQueries` in this file (or anywhere else
 * that touches this data) must go through this factory — per the project's
 * mandatory query-key-factory rule.
 */
export const CompilationTemplateKeys = {
  all: () => ['compilation_template'] as const,
  list: (filters: { search?: string; page?: number; pageSize?: number }) =>
    ['compilation_template', 'list', filters] as const,
  detail: (id: string) => ['compilation_template', 'detail', id] as const,
  builtins: () => ['compilation_template', 'builtins'] as const,
};

export const useListCompilationTemplates = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const { data, isFetching: loading } =
    useQuery<CompilationTemplateListResponse>({
      queryKey: CompilationTemplateKeys.list({
        search: debouncedSearchString,
        page: pagination.current,
        pageSize: pagination.pageSize,
      }),
      initialData: { total: 0, templates: [] },
      gcTime: 0,
      queryFn: async () => {
        const { data } = await compilationTemplateService.list({
          keywords: debouncedSearchString,
          page: pagination.current,
          page_size: pagination.pageSize,
        });
        return data?.data ?? { total: 0, templates: [] };
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

export const useFetchSavedCompilationTemplates = () => {
  const { data, isFetching: loading } =
    useQuery<CompilationTemplateListResponse>({
      queryKey: CompilationTemplateKeys.list({
        search: '',
        page: 1,
        pageSize: 100,
      }),
      initialData: { total: 0, templates: [] },
      queryFn: async () => {
        const { data } = await compilationTemplateService.list({
          keywords: '',
          page: 1,
          page_size: 100,
        });
        return data?.data ?? { total: 0, templates: [] };
      },
    });

  return { data, loading };
};

export const useFetchCompilationTemplate = (id: string) => {
  const { data, isFetching: loading } = useQuery<
    CompilationTemplate | undefined
  >({
    queryKey: CompilationTemplateKeys.detail(id),
    initialData: undefined,
    gcTime: 0,
    enabled: !!id,
    queryFn: async () => {
      const { data } = await compilationTemplateService.get({ id });
      return data?.data;
    },
  });

  return { data, loading, id };
};

/**
 * Cached server-side defaults. Stable across the session — the same
 * factory entry is reused by the editor popover and the seeding helpers.
 */
export const useFetchBuiltinCompilationTemplates = () => {
  const { data, isFetching, isLoading, refetch } = useQuery<
    BuiltinCompilationTemplate[]
  >({
    queryKey: CompilationTemplateKeys.builtins(),
    staleTime: 0,
    refetchOnMount: 'always',
    queryFn: async () => {
      const { data } = await compilationTemplateService.builtins();
      return [...(data?.data ?? [])].sort((a, b) => {
        if (a.kind === 'empty' && b.kind !== 'empty') return 1;
        if (a.kind !== 'empty' && b.kind === 'empty') return -1;
        return a.display_name.localeCompare(b.display_name);
      });
    },
  });

  return { data: data ?? [], loading: isLoading || isFetching, refetch };
};

export const useCreateCompilationTemplate = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['createCompilationTemplate'],
    mutationFn: async (params: ICreateCompilationTemplateRequest) => {
      const { data = {} } = await compilationTemplateService.create(params);
      if (data.code === 0) {
        message.success(i18n.t('message.created'));
        queryClient.invalidateQueries({
          queryKey: CompilationTemplateKeys.all(),
        });
      }
      return data.code;
    },
  });

  return { data, loading, createCompilationTemplate: mutateAsync };
};

export const useUpdateCompilationTemplate = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['updateCompilationTemplate'],
    mutationFn: async (params: IUpdateCompilationTemplateRequest) => {
      const { data = {} } = await compilationTemplateService.update(params);
      if (data.code === 0) {
        message.success(i18n.t('message.updated'));
        queryClient.invalidateQueries({
          queryKey: CompilationTemplateKeys.all(),
        });
      }
      return data.code;
    },
  });

  return { data, loading, updateCompilationTemplate: mutateAsync };
};

export const useDeleteCompilationTemplate = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteCompilationTemplate'],
    mutationFn: async (ids: string[]) => {
      const results = await Promise.all(
        ids.map((id) => compilationTemplateService.delete({ id })),
      );
      const failed = results.find(({ data = {} }) => data.code !== 0);
      const data = failed?.data ?? { code: 0, data: true };
      if (!failed) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: CompilationTemplateKeys.all(),
        });
      }
      return data;
    },
  });

  return { data, loading, deleteCompilationTemplate: mutateAsync };
};
