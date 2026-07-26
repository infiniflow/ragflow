import message from '@/components/ui/message';
import {
  ICompilationTemplate,
  ICompilationTemplateBuiltin,
  ICompilationTemplateListResult,
  ICompilationTemplateSection,
  IWikiPreset,
} from '@/interfaces/database/compilation-template';
import {
  ICreateCompilationTemplateRequestBody,
  IUpdateCompilationTemplateRequestBody,
} from '@/interfaces/request/compilation-template';
import i18n from '@/locales/config';
import compilationTemplateService, {
  createCompilationTemplate,
  deleteCompilationTemplate,
  getCompilationTemplate,
  listBuiltinCompilationTemplates,
  listWikiPresets,
  updateCompilationTemplate,
} from '@/services/compilation-template-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { useCallback, useMemo } from 'react';

import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';

export const enum CompilationTemplateApiAction {
  FetchCompilationTemplates = 'fetchCompilationTemplates',
  FetchCompilationTemplate = 'fetchCompilationTemplate',
  FetchBuiltinCompilationTemplates = 'fetchBuiltinCompilationTemplates',
  CreateCompilationTemplate = 'createCompilationTemplate',
  UpdateCompilationTemplate = 'updateCompilationTemplate',
  DeleteCompilationTemplate = 'deleteCompilationTemplate',
  FetchWikiPresets = 'fetchWikiPresets',
}

export const CompilationTemplateKeys = {
  list: (keywords?: string, page?: number, pageSize?: number) =>
    [
      CompilationTemplateApiAction.FetchCompilationTemplates,
      { keywords, page, pageSize },
    ] as const,
  detail: (id?: string) =>
    [CompilationTemplateApiAction.FetchCompilationTemplate, id] as const,
  builtins: () =>
    [CompilationTemplateApiAction.FetchBuiltinCompilationTemplates] as const,
  all: () => [CompilationTemplateApiAction.FetchCompilationTemplates] as const,
  wikiPresets: () => [CompilationTemplateApiAction.FetchWikiPresets] as const,
};

export const useFetchCompilationTemplatesByPage = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const { data, isFetching: loading } =
    useQuery<ICompilationTemplateListResult>({
      queryKey: CompilationTemplateKeys.list(
        debouncedSearchString,
        pagination.current,
        pagination.pageSize,
      ),
      initialData: {
        templates: [],
        total: 0,
      },
      gcTime: 0,
      queryFn: async () => {
        const { data } = await compilationTemplateService.listTemplates(
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
          templates: (data?.data?.templates ?? []) as ICompilationTemplate[],
          total: data?.data?.total ?? 0,
        };
      },
    });

  const currentPagination = useMemo(
    () => ({ ...pagination, total: data?.total ?? 0 }),
    [pagination, data?.total],
  );

  return {
    templates: data?.templates ?? [],
    total: data?.total ?? 0,
    searchString,
    handleInputChange,
    pagination: currentPagination,
    setPagination,
    loading,
  };
};

export const useFetchCompilationTemplate = (id?: string) => {
  const { data, isFetching: loading } = useQuery<
    ICompilationTemplate | undefined
  >({
    queryKey: CompilationTemplateKeys.detail(id),
    enabled: !!id && id !== 'create',
    gcTime: 0,
    queryFn: async () => {
      if (!id || id === 'create') return undefined;
      const { data } = await getCompilationTemplate(id);
      return data?.data as ICompilationTemplate | undefined;
    },
  });

  return { data, loading };
};

export const useFetchBuiltinCompilationTemplates = () => {
  const { data, isFetching: loading } = useQuery<ICompilationTemplateBuiltin[]>(
    {
      queryKey: CompilationTemplateKeys.builtins(),
      initialData: [],
      gcTime: 0,
      queryFn: async () => {
        const { data } = await listBuiltinCompilationTemplates();
        return (data?.data ?? []) as ICompilationTemplateBuiltin[];
      },
    },
  );

  const kindOptions = useMemo(() => {
    const kindSet = new Set<string>();
    (data ?? []).forEach((template) => {
      if (template?.kind) kindSet.add(template.kind);
    });
    return Array.from(kindSet)
      .sort()
      .map((value) => ({ label: value, value }));
  }, [data]);

  const typeOptions = useMemo(() => {
    const typeSet = new Set<string>();
    (data ?? []).forEach((template) => {
      Object.entries(template?.config ?? {}).forEach(([key, section]) => {
        if (['kind', 'llm_id', 'global_rules'].includes(key)) return;
        (section as ICompilationTemplateSection)?.fields?.forEach((field) => {
          if (field?.type) typeSet.add(field.type);
        });
      });
    });
    return Array.from(typeSet)
      .sort()
      .map((value) => ({ label: value, value }));
  }, [data]);

  return { data, typeOptions, kindOptions, loading };
};

export const useCreateCompilationTemplate = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [CompilationTemplateApiAction.CreateCompilationTemplate],
    mutationFn: async (params: ICreateCompilationTemplateRequestBody) => {
      const { data } = await createCompilationTemplate(params);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [CompilationTemplateApiAction.FetchCompilationTemplates],
      });
    },
  });

  const createTemplate = useCallback(
    async (params: ICreateCompilationTemplateRequestBody) => {
      const result = await mutateAsync(params);
      if (result.code === 0) {
        message.success(i18n.t('message.created'));
      }
      return result;
    },
    [mutateAsync],
  );

  return { data, loading, createTemplate };
};

export const useUpdateCompilationTemplate = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [CompilationTemplateApiAction.UpdateCompilationTemplate],
    mutationFn: async ({
      id,
      params,
    }: {
      id: string;
      params: IUpdateCompilationTemplateRequestBody;
    }) => {
      const { data } = await updateCompilationTemplate(id, params);
      return data;
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: [CompilationTemplateApiAction.FetchCompilationTemplates],
      });
      queryClient.invalidateQueries({
        queryKey: CompilationTemplateKeys.detail(variables.id),
      });
    },
  });

  const updateTemplate = useCallback(
    async (id: string, params: IUpdateCompilationTemplateRequestBody) => {
      const result = await mutateAsync({ id, params });
      if (result.code === 0) {
        message.success(i18n.t('message.modified'));
      }
      return result;
    },
    [mutateAsync],
  );

  return { data, loading, updateTemplate };
};

export const useDeleteCompilationTemplate = () => {
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [CompilationTemplateApiAction.DeleteCompilationTemplate],
    mutationFn: async (id: string) => {
      const { data } = await deleteCompilationTemplate(id);
      if (data.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: [CompilationTemplateApiAction.FetchCompilationTemplates],
        });
      }
      return data?.data ?? true;
    },
  });

  const deleteTemplate = useCallback(
    async (id: string) => {
      await mutateAsync(id);
    },
    [mutateAsync],
  );

  return { data, loading, deleteTemplate };
};

export const useFetchAllCompilationTemplates = () => {
  const { data, isFetching: loading } = useQuery<ICompilationTemplate[]>({
    queryKey: CompilationTemplateKeys.all(),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await compilationTemplateService.listTemplates(
        {
          params: { keywords: '', page: 1, page_size: 100 },
        },
        true,
      );
      return (data?.data?.templates ?? []) as ICompilationTemplate[];
    },
  });

  return { templates: data ?? [], loading };
};

export const useFetchWikiPresets = () => {
  const { data, isFetching: loading } = useQuery<IWikiPreset[]>({
    queryKey: CompilationTemplateKeys.wikiPresets(),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listWikiPresets();
      return (data?.data ?? []) as IWikiPreset[];
    },
  });

  return { data: data ?? [], loading };
};
