import message from '@/components/ui/message';
import {
  ICompilationTemplate,
  ICompilationTemplateListResult,
} from '@/interfaces/database/compilation-template';
import i18n from '@/locales/config';
import compilationTemplateService, {
  deleteCompilationTemplate,
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
  DeleteCompilationTemplate = 'deleteCompilationTemplate',
}

export const CompilationTemplateKeys = {
  list: (keywords?: string, page?: number, pageSize?: number) =>
    [
      CompilationTemplateApiAction.FetchCompilationTemplates,
      { keywords, page, pageSize },
    ] as const,
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
