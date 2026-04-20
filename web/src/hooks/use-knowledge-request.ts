import { useHandleFilterSubmit } from '@/components/list-filter-bar/use-handle-filter-submit';
import message from '@/components/ui/message';
import { ParseType } from '@/constants/knowledge';
import { ResponsePostType } from '@/interfaces/database/base';
import { IDataset, IDatasetListResult } from '@/interfaces/database/dataset';
import {
  IKnowledge,
  IKnowledgeGraph,
  INextTestingResult,
  IRenameTag,
  ITestingResult,
} from '@/interfaces/database/knowledge';
import { ITestRetrievalRequestBody } from '@/interfaces/request/knowledge';
import i18n from '@/locales/config';
import kbService, {
  deleteKnowledgeGraph,
  getKnowledgeGraph,
  listDataset,
  listTag,
  removeTag,
  renameTag,
  updateKb,
} from '@/services/knowledge-service';
import {
  useIsMutating,
  useMutation,
  useMutationState,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { omit } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { useParams, useSearchParams } from 'react-router';
import {
  useGetPaginationWithRouter,
  useHandleSearchChange,
} from './logic-hooks';
import { useSetPaginationParams } from './route-hook';

export const enum KnowledgeApiAction {
  TestRetrieval = 'testRetrieval',
  FetchKnowledgeListByPage = 'fetchKnowledgeListByPage',
  CreateKnowledge = 'createKnowledge',
  DeleteKnowledge = 'deleteKnowledge',
  SaveKnowledge = 'saveKnowledge',
  FetchKnowledgeDetail = 'fetchKnowledgeDetail',
  FetchKnowledgeGraph = 'fetchKnowledgeGraph',
  FetchMetadata = 'fetchMetadata',
  FetchMetadataKeys = 'fetchMetadataKeys',
  FetchKnowledgeList = 'fetchKnowledgeList',
  RemoveKnowledgeGraph = 'removeKnowledgeGraph',
}

export const useKnowledgeBaseId = (): string => {
  const { id } = useParams();

  return (id as string) || '';
};

export const useTestRetrieval = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const [values, setValues] = useState<ITestRetrievalRequestBody>();
  const { filterValue, setFilterValue } = useHandleFilterSubmit();

  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const queryParams = useMemo(() => {
    return {
      ...values,
      kb_id: values?.kb_id || knowledgeBaseId,
      page,
      size: pageSize,
      doc_ids: filterValue.doc_ids,
      highlight: true,
    };
  }, [filterValue, knowledgeBaseId, page, pageSize, values]);

  const mutation = useMutation<INextTestingResult, Error, typeof queryParams>({
    mutationFn: async (params) => {
      const { data } = await kbService.retrievalTest(params);
      const result = data?.data ?? {};
      return { ...result, isRuned: true };
    },
  });

  const refetch = useCallback(() => {
    if (queryParams.question) {
      mutation.mutate(queryParams);
    }
  }, [mutation, queryParams]);

  const onPaginationChange = useCallback(
    (newPage: number, newPageSize: number) => {
      setPage(newPage);
      setPageSize(newPageSize);
      if (mutation.data && queryParams.question) {
        const newParams = { ...queryParams, page: newPage, size: newPageSize };
        mutation.mutate(newParams);
      }
    },
    [mutation, queryParams],
  );

  const handleFilterSubmit = useCallback(
    (value: { doc_ids?: string[] }) => {
      setFilterValue(value);
      setPage(1);
      if (mutation.data && queryParams.question) {
        const newParams = {
          ...queryParams,
          doc_ids: value.doc_ids ?? [],
          page: 1,
        };
        mutation.mutate(newParams);
      }
    },
    [mutation, queryParams, setFilterValue],
  );

  const data = useMemo(
    () =>
      mutation.data ?? {
        chunks: [],
        doc_aggs: [],
        total: 0,
        isRuned: false,
      },
    [mutation.data],
  );

  return {
    data,
    loading: mutation.isPending,
    setValues,
    refetch,
    onPaginationChange,
    page,
    pageSize,
    handleFilterSubmit,
    filterValue,
  };
};

export const useFetchNextKnowledgeListByPage = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });
  const { filterValue, handleFilterSubmit } = useHandleFilterSubmit();

  const { data, isFetching: loading } = useQuery<IDatasetListResult>({
    queryKey: [
      KnowledgeApiAction.FetchKnowledgeListByPage,
      {
        debouncedSearchString,
        ...pagination,
        filterValue,
      },
    ],
    initialData: {
      kbs: [],
      total_datasets: 0,
    },
    gcTime: 0,
    queryFn: async () => {
      const { data } = await listDataset({
        page_size: pagination.pageSize,
        page: pagination.current,
        ext: {
          keywords: debouncedSearchString,
          owner_ids: filterValue.owner as string[],
        },
      });

      return { kbs: data?.data, total_datasets: data?.total_datasets };
    },
  });

  const onInputChange: React.ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      // setPagination({ page: 1 }); // TODO: This results in repeated requests
      handleInputChange(e);
    },
    [handleInputChange],
  );

  return {
    ...data,
    searchString,
    handleInputChange: onInputChange,
    pagination: { ...pagination, total: data?.total_datasets },
    setPagination,
    loading,
    filterValue,
    handleFilterSubmit,
  };
};

export const useCreateKnowledge = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [KnowledgeApiAction.CreateKnowledge],
    mutationFn: async (params: {
      id?: string;
      name: string;
      embedding_model?: string;
      chunk_method?: string;
      parseType?: ParseType;
      pipeline_id?: string | null;
      ext?: {
        language?: string;
        [key: string]: any;
      };
    }) => {
      const { data = {} } = await kbService.createKb(params);
      if (data.code === 0) {
        message.success(
          i18n.t(`message.${params?.id ? 'modified' : 'created'}`),
        );
        queryClient.invalidateQueries({ queryKey: ['fetchKnowledgeList'] });
      }
      return data;
    },
  });

  return { data, loading, createKnowledge: mutateAsync };
};

export const useDeleteKnowledge = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [KnowledgeApiAction.DeleteKnowledge],
    mutationFn: async (id: string) => {
      const { data } = await kbService.rmKb({ ids: [id] });
      if (data.code === 0) {
        message.success(i18n.t(`message.deleted`));
        queryClient.invalidateQueries({
          queryKey: [KnowledgeApiAction.FetchKnowledgeListByPage],
        });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteKnowledge: mutateAsync };
};

export const useUpdateKnowledge = (shouldFetchList = false) => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const queryClient = useQueryClient();

  const extractRaptorConfigExt = (
    raptorConfig: Record<string, any> | undefined,
  ) => {
    if (!raptorConfig) return raptorConfig;
    const {
      use_raptor,
      prompt,
      max_token,
      threshold,
      max_cluster,
      random_seed,
      auto_disable_for_structured_data,
      ext,
      ...raptorExt
    } = raptorConfig;
    return {
      use_raptor,
      prompt,
      max_token,
      threshold,
      max_cluster,
      random_seed,
      auto_disable_for_structured_data,
      ext: { ...ext, ...raptorExt },
    };
  };

  const extractParserConfigExt = (
    parserConfig: Record<string, any> | undefined,
  ) => {
    if (!parserConfig) return parserConfig;
    const {
      auto_keywords,
      auto_questions,
      chunk_token_num,
      delimiter,
      graphrag,
      html4excel,
      layout_recognize,
      raptor,
      tag_kb_ids,
      topn_tags,
      filename_embd_weight,
      task_page_size,
      pages,
      children_delimiter,
      use_parent_child,
      enable_children,
      ext,
      ...parserExt
    } = parserConfig;
    return {
      auto_keywords,
      auto_questions,
      chunk_token_num,
      delimiter,
      graphrag,
      html4excel,
      layout_recognize,
      raptor: extractRaptorConfigExt(raptor),
      tag_kb_ids,
      topn_tags,
      filename_embd_weight,
      task_page_size,
      pages,
      parent_child: enable_children
        ? {
            children_delimiter,
            use_parent_child: use_parent_child ?? enable_children,
          }
        : undefined,
      ext: { ...ext, ...parserExt },
    };
  };

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [KnowledgeApiAction.SaveKnowledge],
    mutationFn: async (params: {
      kb_id?: string;
      name?: string;
      embedding_model?: string;
      chunk_method?: string;
      pipeline_id?: string | null;
      avatar?: string | null;
      description?: string;
      permission?: string;
      pagerank?: number;
      parser_config?: Record<string, any>;
      [key: string]: any;
    }) => {
      const kbId = params?.kb_id || knowledgeBaseId;
      const {
        embedding_model,
        chunk_method,
        pipeline_id,
        avatar,
        description,
        permission,
        pagerank,
        parser_config,
        ...ext
      } = params;
      const requestBody: Record<string, any> = {
        name,
        embedding_model,
        chunk_method,
        pipeline_id,
        avatar,
        description,
        permission,
        pagerank,
        parser_config: extractParserConfigExt(parser_config),
        ...omit(ext, ['kb_id']),
      };
      const { data = {} } = await updateKb(kbId, requestBody);
      if (data.code === 0) {
        message.success(i18n.t(`message.updated`));
        if (shouldFetchList) {
          queryClient.invalidateQueries({
            queryKey: [KnowledgeApiAction.FetchKnowledgeListByPage],
          });
        } else {
          queryClient.invalidateQueries({ queryKey: ['fetchKnowledgeDetail'] });
        }
      }
      return data;
    },
  });

  return { data, loading, saveKnowledgeConfiguration: mutateAsync };
};

export const useFetchKnowledgeBaseConfiguration = (props?: {
  isEdit?: boolean;
}) => {
  const { isEdit = true } = props || { isEdit: true };
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id') || id;

  const { data, isFetching: loading } = useQuery<IKnowledge>({
    queryKey: [KnowledgeApiAction.FetchKnowledgeDetail, knowledgeBaseId],
    initialData: {} as IKnowledge,
    gcTime: 0,
    enabled: !!knowledgeBaseId && isEdit,
    queryFn: async () => {
      const { data } = await kbService.getKbDetail({
        kb_id: knowledgeBaseId,
      });
      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export function useFetchKnowledgeGraph() {
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<IKnowledgeGraph>({
    queryKey: [KnowledgeApiAction.FetchKnowledgeGraph, knowledgeBaseId],
    initialData: { graph: {}, mind_map: {} } as IKnowledgeGraph,
    enabled: !!knowledgeBaseId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await getKnowledgeGraph(knowledgeBaseId);
      return data?.data;
    },
  });

  return { data, loading };
}

export function useFetchKnowledgeMetadata(kbIds: string[] = []) {
  const { data, isFetching: loading } = useQuery<
    Record<string, Record<string, string[]>>
  >({
    queryKey: [KnowledgeApiAction.FetchMetadata, kbIds],
    initialData: {},
    enabled: kbIds.length > 0,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await kbService.getMeta({ kb_ids: kbIds.join(',') });
      return data?.data ?? {};
    },
  });

  return { data, loading };
}

export function useFetchKnowledgeMetadataKeys(kbIds: string[] = []) {
  const { data, isFetching: loading } = useQuery<string[]>({
    queryKey: [KnowledgeApiAction.FetchMetadataKeys, kbIds],
    initialData: [],
    enabled: kbIds.length > 0,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await kbService.getMetaKeys({ kb_ids: kbIds.join(',') });
      return data?.data ?? [];
    },
  });

  return { data, loading };
}

export const useRemoveKnowledgeGraph = () => {
  const knowledgeBaseId = useKnowledgeBaseId();

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [KnowledgeApiAction.RemoveKnowledgeGraph],
    mutationFn: async () => {
      const { data } = await deleteKnowledgeGraph(knowledgeBaseId);
      if (data.code === 0) {
        message.success(i18n.t(`message.deleted`));
        queryClient.invalidateQueries({
          queryKey: [KnowledgeApiAction.FetchKnowledgeGraph],
        });
      }
      return data?.code;
    },
  });

  return { data, loading, removeKnowledgeGraph: mutateAsync };
};

export const useFetchKnowledgeList = (
  shouldFilterListWithoutDocument: boolean = false,
): {
  list: IDataset[];
  loading: boolean;
} => {
  const { data, isFetching: loading } = useQuery({
    queryKey: [KnowledgeApiAction.FetchKnowledgeList],
    initialData: [],
    gcTime: 0, // https://tanstack.com/query/latest/docs/framework/react/guides/caching?from=reactQueryV3
    queryFn: async () => {
      const { data } = await listDataset();
      const list = data?.data ?? [];
      return shouldFilterListWithoutDocument
        ? list.filter((x: IDataset) => x.chunk_count > 0)
        : list;
    },
  });

  return { list: data, loading };
};

export const useSelectKnowledgeOptions = () => {
  const { list } = useFetchKnowledgeList();

  const options = list?.map((item) => ({
    label: item.name,
    value: item.id,
  }));

  return options;
};

//#region tags
export const useRenameTag = () => {
  const knowledgeBaseId = useKnowledgeBaseId();

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['renameTag'],
    mutationFn: async (params: IRenameTag) => {
      const { data } = await renameTag(knowledgeBaseId, params);
      if (data.code === 0) {
        message.success(i18n.t(`message.modified`));
        queryClient.invalidateQueries({
          queryKey: ['fetchTagList'],
        });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, renameTag: mutateAsync };
};

export const useTagIsRenaming = () => {
  return useIsMutating({ mutationKey: ['renameTag'] }) > 0;
};

export const useFetchTagListByKnowledgeIds = () => {
  const [knowledgeIds, setKnowledgeIds] = useState<string[]>([]);

  const { data, isFetching: loading } = useQuery<Array<[string, number]>>({
    queryKey: ['fetchTagListByKnowledgeIds'],
    enabled: knowledgeIds.length > 0,
    initialData: [],
    gcTime: 0, // https://tanstack.com/query/latest/docs/framework/react/guides/caching?from=reactQueryV3
    queryFn: async () => {
      const { data } = await kbService.listTagByKnowledgeIds({
        kb_ids: knowledgeIds.join(','),
      });
      const list = data?.data || [];
      return list;
    },
  });

  return { list: data, loading, setKnowledgeIds };
};

export const useFetchTagList = () => {
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<Array<[string, number]>>({
    queryKey: ['fetchTagList'],
    initialData: [],
    gcTime: 0, // https://tanstack.com/query/latest/docs/framework/react/guides/caching?from=reactQueryV3
    queryFn: async () => {
      const { data } = await listTag(knowledgeBaseId);
      const list = data?.data || [];
      return list;
    },
  });

  return { list: data, loading };
};

export const useDeleteTag = () => {
  const knowledgeBaseId = useKnowledgeBaseId();

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['deleteTag'],
    mutationFn: async (tags: string[]) => {
      const { data } = await removeTag(knowledgeBaseId, tags);
      if (data.code === 0) {
        message.success(i18n.t(`message.deleted`));
        queryClient.invalidateQueries({
          queryKey: ['fetchTagList'],
        });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteTag: mutateAsync };
};

// #endregion

//#region Retrieval testing

export const useTestChunkRetrieval = (): ResponsePostType<ITestingResult> & {
  testChunk: (...params: any[]) => void;
} => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const { page, size: pageSize } = useSetPaginationParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['testChunk'], // This method is invalid
    gcTime: 0,
    mutationFn: async (values: any) => {
      const { data } = await kbService.retrievalTest({
        ...values,
        kb_id: values.kb_id ?? knowledgeBaseId,
        highlight: true,
        page,
        size: pageSize,
      });
      if (data.code === 0) {
        const res = data.data;
        return {
          ...res,
          documents: res.doc_aggs,
        };
      }
      return (
        data?.data ?? {
          chunks: [],
          documents: [],
          total: 0,
        }
      );
    },
  });

  return {
    data: data ?? { chunks: [], documents: [], total: 0 },
    loading,
    testChunk: mutateAsync,
  };
};

export const useTestChunkAllRetrieval = (): ResponsePostType<ITestingResult> & {
  testChunkAll: (...params: any[]) => void;
} => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const { page, size: pageSize } = useSetPaginationParams();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['testChunkAll'], // This method is invalid
    gcTime: 0,
    mutationFn: async (values: any) => {
      const { data } = await kbService.retrievalTest({
        ...values,
        kb_id: values.kb_id ?? knowledgeBaseId,
        highlight: true,
        doc_ids: [],
        page,
        size: pageSize,
      });
      if (data.code === 0) {
        const res = data.data;
        return {
          ...res,
          documents: res.doc_aggs,
        };
      }
      return (
        data?.data ?? {
          chunks: [],
          documents: [],
          total: 0,
        }
      );
    },
  });

  return {
    data: data ?? { chunks: [], documents: [], total: 0 },
    loading,
    testChunkAll: mutateAsync,
  };
};

export const useChunkIsTesting = () => {
  return useIsMutating({ mutationKey: ['testChunk'] }) > 0;
};

export const useSelectTestingResult = (): ITestingResult => {
  const data = useMutationState({
    filters: { mutationKey: ['testChunk'] },
    select: (mutation) => {
      return mutation.state.data;
    },
  });
  return (data.at(-1) ?? {
    chunks: [],
    documents: [],
    total: 0,
  }) as ITestingResult;
};

export const useSelectIsTestingSuccess = () => {
  const status = useMutationState({
    filters: { mutationKey: ['testChunk'] },
    select: (mutation) => {
      return mutation.state.status;
    },
  });
  return status.at(-1) === 'success';
};

export const useAllTestingSuccess = () => {
  const status = useMutationState({
    filters: { mutationKey: ['testChunkAll'] },
    select: (mutation) => {
      return mutation.state.status;
    },
  });
  return status.at(-1) === 'success';
};

export const useAllTestingResult = (): ITestingResult => {
  const data = useMutationState({
    filters: { mutationKey: ['testChunkAll'] },
    select: (mutation) => {
      return mutation.state.data;
    },
  });
  return (data.at(-1) ?? {
    chunks: [],
    documents: [],
    total: 0,
  }) as ITestingResult;
};
//#endregion
