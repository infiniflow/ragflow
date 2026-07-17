import { useHandleFilterSubmit } from '@/components/list-filter-bar/use-handle-filter-submit';
import message from '@/components/ui/message';
import { ParseType } from '@/constants/knowledge';
import { ResponsePostType, ResponseType } from '@/interfaces/database/base';
import {
  IArtifact,
  IArtifactGraph,
  IArtifactPage,
  IArtifactTopic,
  IDataset,
  IDatasetListResult,
  IKnowledgeGraph,
  INextTestingResult,
  IRenameTag,
  ITestingResult,
  IWikiCommit,
  IWikiCommitDetail,
  IWikiCommitListResponse,
} from '@/interfaces/database/dataset';
import {
  IFetchArtifactGraphRequestParams,
  ITestRetrievalRequestBody,
  IUpdateArtifactPageRequestParams,
} from '@/interfaces/request/knowledge';
import i18n from '@/locales/config';
import kbService, {
  clearWiki,
  deleteKnowledgeGraph,
  getArtifactGraph,
  getArtifactPage,
  getKbDetail,
  getKnowledgeGraph,
  getWikiCommit,
  listArtifactTopics,
  listArtifacts,
  listDataset,
  listTag,
  listWikiCommits,
  removeTag,
  renameTag,
  updateArtifactPage,
  updateKb,
} from '@/services/knowledge-service';
import {
  useInfiniteQuery,
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
import { extractParserConfigExt } from './parser-config-utils';
import { useSetPaginationParams } from './route-hook';

export const enum KnowledgeApiAction {
  FetchKnowledgeListByPage = 'fetchKnowledgeListByPage',
  CreateKnowledge = 'createKnowledge',
  DeleteKnowledge = 'deleteKnowledge',
  SaveKnowledge = 'saveKnowledge',
  FetchKnowledgeDetail = 'fetchKnowledgeDetail',
  FetchKnowledgeGraph = 'fetchKnowledgeGraph',
  FetchArtifactList = 'fetchArtifactList',
  FetchArtifactTopicList = 'fetchArtifactTopicList',
  FetchArtifactPage = 'fetchArtifactPage',
  FetchArtifactGraph = 'fetchArtifactGraph',
  UpdateArtifactPage = 'updateArtifactPage',
  FetchWikiCommits = 'fetchWikiCommits',
  FetchWikiCommit = 'fetchWikiCommit',
  FetchMetadata = 'fetchMetadata',
  FetchMetadataKeys = 'fetchMetadataKeys',
  FetchKnowledgeList = 'fetchKnowledgeList',
  RemoveKnowledgeGraph = 'removeKnowledgeGraph',
  ClearWiki = 'clearWiki',
}

export const useKnowledgeBaseId = (): string => {
  const { id } = useParams();

  return (id as string) || '';
};

export const useTestRetrieval = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const [values, setValues] = useState<ITestRetrievalRequestBody>();
  const { filterValue, setFilterValue } = useHandleFilterSubmit();

  const queryParams = useMemo(() => {
    return {
      ...values,
      kb_id: values?.kb_id || knowledgeBaseId,
      page: 1,
      doc_ids: filterValue.doc_ids,
      highlight: true,
    };
  }, [filterValue, knowledgeBaseId, values]);

  const mutation = useMutation<INextTestingResult, Error, typeof queryParams>({
    mutationFn: async (params) => {
      const { data } = await kbService.retrievalTest(params);
      const result = data?.data ?? {};
      return { ...result, isRuned: true };
    },
  });

  const refetch = useCallback(() => {
    if (queryParams.question) {
      const newParams = { ...queryParams, page: 1 };
      mutation.mutate(newParams);
    }
  }, [mutation, queryParams]);

  const handleFilterSubmit = useCallback(
    (value: { doc_ids?: string[] }) => {
      setFilterValue(value);
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

  const { data, isFetching: loading } = useQuery<IDataset>({
    queryKey: [KnowledgeApiAction.FetchKnowledgeDetail, knowledgeBaseId],
    initialData: {} as IDataset,
    gcTime: 0,
    enabled: !!knowledgeBaseId && isEdit,
    queryFn: async () => {
      const { data } = await getKbDetail(knowledgeBaseId || '');
      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const ArtifactKeys = {
  list: (
    datasetId: string,
    keywords: string,
    topic?: string,
    pageType?: string,
  ) =>
    [
      KnowledgeApiAction.FetchArtifactList,
      datasetId,
      keywords,
      topic,
      pageType,
    ] as const,
  listByDataset: (datasetId: string) =>
    [KnowledgeApiAction.FetchArtifactList, datasetId] as const,
  detail: (datasetId: string, pageType: string, slug: string) =>
    [KnowledgeApiAction.FetchArtifactPage, datasetId, pageType, slug] as const,
};

export const ArtifactTopicKeys = {
  list: (datasetId: string, keywords: string) =>
    [KnowledgeApiAction.FetchArtifactTopicList, datasetId, keywords] as const,
  listByDataset: (datasetId: string) =>
    [KnowledgeApiAction.FetchArtifactTopicList, datasetId] as const,
};

const wikiCommitKeys = {
  list: (datasetId: string, pageType: string, slug: string) =>
    [KnowledgeApiAction.FetchWikiCommits, datasetId, pageType, slug] as const,
  detail: (datasetId: string, commitId: string) =>
    [KnowledgeApiAction.FetchWikiCommit, datasetId, commitId] as const,
};

export const useFetchWikiCommits = (
  artifact: IArtifact | null,
  enabled = true,
) => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const pageType = artifact?.page_type ?? '';
  const slug = artifact?.slug ?? '';

  const { data, isFetching: loading } =
    useQuery<IWikiCommitListResponse | null>({
      queryKey: wikiCommitKeys.list(knowledgeBaseId, pageType, slug),
      enabled:
        !!knowledgeBaseId && !!artifact && !!pageType && !!slug && enabled,
      gcTime: 0,
      queryFn: async () => {
        const { data } = await listWikiCommits(knowledgeBaseId, pageType, slug);
        // The merged file-commit endpoint returns {total, page, page_size, commits},
        // while the existing components expect {total, items}. Normalize here.
        const raw = (data?.data ?? {}) as {
          total?: number;
          items?: IWikiCommit[];
          commits?: IWikiCommit[];
        };
        return {
          total: raw.total ?? 0,
          items: raw.items ?? raw.commits ?? [],
        };
      },
    });

  return {
    commits: data?.items ?? [],
    total: data?.total ?? 0,
    loading,
  };
};

export function useFetchWikiCommit(commitId: string | null) {
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<IWikiCommitDetail | null>({
    queryKey: wikiCommitKeys.detail(knowledgeBaseId, commitId ?? ''),
    enabled: !!knowledgeBaseId && !!commitId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await getWikiCommit(knowledgeBaseId, commitId!);
      return data?.data ?? null;
    },
  });

  return { data, loading };
}

type UseFetchArtifactListOptions = {
  keywords?: string;
  topic?: string;
  pageType?: string;
  enabled?: boolean;
};

export const useFetchArtifactList = (
  options: UseFetchArtifactListOptions = {},
) => {
  const { keywords = '', topic, pageType, enabled = true } = options;
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, fetchNextPage, hasNextPage, isFetching, isFetchingNextPage } =
    useInfiniteQuery<{
      artifacts: IArtifact[];
      total: number;
    }>({
      queryKey: ArtifactKeys.list(knowledgeBaseId, keywords, topic, pageType),
      enabled: !!knowledgeBaseId && enabled && !!topic,
      gcTime: 0,
      initialPageParam: 1,
      queryFn: async ({ pageParam }) => {
        const page = pageParam as number;
        const { data } = await listArtifacts(knowledgeBaseId, {
          page,
          page_size: 30,
          keywords,
          topic,
          page_type: pageType,
        });

        const responseData = data?.data;

        return {
          artifacts: responseData?.items ?? [],
          total: responseData?.total ?? 0,
        };
      },
      getNextPageParam: (lastPage, allPages) => {
        const loadedCount = allPages.reduce(
          (sum, page) => sum + page.artifacts.length,
          0,
        );
        return loadedCount < lastPage.total ? allPages.length + 1 : undefined;
      },
    });

  const artifacts = useMemo(
    () => data?.pages.flatMap((page) => page.artifacts) ?? [],
    [data],
  );

  const loading = isFetching || isFetchingNextPage;

  const handleScroll = useCallback(
    (e: React.UIEvent<HTMLDivElement>) => {
      const { scrollTop, scrollHeight, clientHeight } = e.currentTarget;
      const threshold = 50;
      if (
        scrollHeight - scrollTop - clientHeight <= threshold &&
        hasNextPage &&
        !isFetchingNextPage
      ) {
        fetchNextPage();
      }
    },
    [fetchNextPage, hasNextPage, isFetchingNextPage],
  );

  return {
    artifacts,
    loading,
    handleScroll,
    hasMore: !!hasNextPage,
  };
};

type UseFetchArtifactTopicListOptions = {
  keywords?: string;
  enabled?: boolean;
};

export const useFetchArtifactTopicList = (
  options: UseFetchArtifactTopicListOptions = {},
) => {
  const { keywords = '', enabled = true } = options;
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, fetchNextPage, hasNextPage, isFetching, isFetchingNextPage } =
    useInfiniteQuery<{
      topics: IArtifactTopic[];
      total: number;
    }>({
      queryKey: ArtifactTopicKeys.list(knowledgeBaseId, keywords),
      enabled: !!knowledgeBaseId && enabled,
      gcTime: 0,
      initialPageParam: 1,
      queryFn: async ({ pageParam }) => {
        const page = pageParam as number;
        const { data } = await listArtifactTopics(knowledgeBaseId, {
          page,
          page_size: 30,
          keywords,
        });

        const responseData = data?.data;

        return {
          topics: responseData?.items ?? [],
          total: responseData?.total ?? 0,
        };
      },
      getNextPageParam: (lastPage, allPages) => {
        const loadedCount = allPages.reduce(
          (sum, page) => sum + page.topics.length,
          0,
        );
        return loadedCount < lastPage.total ? allPages.length + 1 : undefined;
      },
    });

  const topics = useMemo(
    () => data?.pages.flatMap((page) => page.topics) ?? [],
    [data],
  );

  const loading = isFetching || isFetchingNextPage;

  const handleScroll = useCallback(
    (e: React.UIEvent<HTMLDivElement>) => {
      const { scrollTop, scrollHeight, clientHeight } = e.currentTarget;
      const threshold = 50;
      if (
        scrollHeight - scrollTop - clientHeight <= threshold &&
        hasNextPage &&
        !isFetchingNextPage
      ) {
        fetchNextPage();
      }
    },
    [fetchNextPage, hasNextPage, isFetchingNextPage],
  );

  return {
    topics,
    loading,
    handleScroll,
    hasMore: !!hasNextPage,
  };
};

export function useFetchArtifactPage(
  artifact: IArtifact | null,
  enabled = true,
) {
  const knowledgeBaseId = useKnowledgeBaseId();
  const pageType = artifact?.page_type ?? '';
  const slug = artifact?.slug ?? '';

  const { data, isFetching: loading } = useQuery<IArtifactPage | null>({
    queryKey: ArtifactKeys.detail(knowledgeBaseId, pageType, slug),
    enabled: !!knowledgeBaseId && !!artifact && !!pageType && !!slug && enabled,
    queryFn: async () => {
      const { data } = await getArtifactPage(knowledgeBaseId, pageType, slug);
      return data?.data ?? null;
    },
  });

  return { data, loading };
}

export const useUpdateArtifactPage = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation<
    ResponseType<IArtifactPage>,
    Error,
    IUpdateArtifactPageRequestParams
  >({
    mutationKey: [KnowledgeApiAction.UpdateArtifactPage],
    mutationFn: async (params) => {
      const { data = {} } = await updateArtifactPage(
        knowledgeBaseId,
        params.pageType,
        params.slug,
        params.body,
      );
      if (data.code === 0) {
        message.success(i18n.t(`message.updated`));
        queryClient.invalidateQueries({
          queryKey: ArtifactKeys.detail(
            knowledgeBaseId,
            params.pageType,
            params.slug,
          ),
        });
      }
      return data;
    },
  });

  return { data, loading, updateArtifactPage: mutateAsync };
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

export const artifactGraphKeys = {
  graph: (datasetId: string, params?: IFetchArtifactGraphRequestParams) =>
    [KnowledgeApiAction.FetchArtifactGraph, datasetId, params?.node] as const,
};

export function useFetchArtifactGraph(
  params?: IFetchArtifactGraphRequestParams,
  options?: { enabled?: boolean },
) {
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<IArtifactGraph>({
    queryKey: artifactGraphKeys.graph(knowledgeBaseId, params),
    initialData: { entities: [], relations: [] } as IArtifactGraph,
    enabled: !!knowledgeBaseId && (options?.enabled ?? true),
    gcTime: 0,
    queryFn: async () => {
      const { data } = await getArtifactGraph(knowledgeBaseId, params);
      return data?.data ?? { entities: [], relations: [] };
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
      const { data } = await kbService.getMeta({
        dataset_ids: kbIds.join(','),
      });
      return data?.data ?? {};
    },
  });

  return { data, loading };
}

export function useFetchKnowledgeMetadataKeys(kbIds: string[] = []) {
  const sortedKbIds = useMemo(() => [...kbIds].sort(), [kbIds]);
  const { data, isFetching: loading } = useQuery<string[]>({
    queryKey: [KnowledgeApiAction.FetchMetadataKeys, sortedKbIds],
    initialData: [],
    enabled: sortedKbIds.length > 0,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await kbService.getMetaKeys({
        kb_ids: sortedKbIds.join(','),
      });
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

export const useClearWiki = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const queryClient = useQueryClient();

  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [KnowledgeApiAction.ClearWiki],
    mutationFn: async () => {
      const { data } = await clearWiki(knowledgeBaseId);
      if (data?.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: ArtifactKeys.listByDataset(knowledgeBaseId),
        });
        queryClient.invalidateQueries({
          queryKey: ArtifactTopicKeys.listByDataset(knowledgeBaseId),
        });
        queryClient.invalidateQueries({
          queryKey: artifactGraphKeys.graph(knowledgeBaseId),
        });
      }
      return data;
    },
  });

  return { data, loading, clearWiki: mutateAsync };
};

export const useFetchKnowledgeList = (
  shouldFilterListWithoutDocument: boolean = false,
  keywords = '',
): {
  list: IDataset[];
  loading: boolean;
} => {
  const { data, isFetching: loading } = useQuery({
    queryKey: [
      KnowledgeApiAction.FetchKnowledgeList,
      shouldFilterListWithoutDocument,
      keywords,
    ],
    initialData: [],
    gcTime: 0, // https://tanstack.com/query/latest/docs/framework/react/guides/caching?from=reactQueryV3
    queryFn: async () => {
      const { data } = await listDataset(
        keywords
          ? {
              ext: {
                keywords,
              },
            }
          : undefined,
      );
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
    queryKey: ['fetchTagListByKnowledgeIds', knowledgeIds],
    enabled: knowledgeIds.length > 0,
    initialData: [],
    gcTime: 0, // https://tanstack.com/query/latest/docs/framework/react/guides/caching?from=reactQueryV3
    queryFn: async () => {
      const { data } = await kbService.listTagByKnowledgeIds({
        dataset_ids: knowledgeIds.join(','),
      });
      const list = (data?.data || []) as Array<
        [string, number] | { value?: string; count?: number }
      >;
      return list.flatMap((tag): Array<[string, number]> => {
        if (Array.isArray(tag)) {
          return [tag];
        }
        return tag.value ? [[tag.value, tag.count ?? 0]] : [];
      });
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
