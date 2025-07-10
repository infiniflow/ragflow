import { ResponsePostType } from '@/interfaces/database/base';
import {
  IKnowledge,
  IKnowledgeGraph,
  IRenameTag,
  ITestingResult,
} from '@/interfaces/database/knowledge';
import i18n from '@/locales/config';
import kbService, {
  deleteKnowledgeGraph,
  getKnowledgeGraph,
  listDataset,
  listTag,
  removeTag,
  renameTag,
  resolveEntities,
  detectCommunities,
  getCommunityDetectionProgress,
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
import { message } from 'antd';
import { useState, useEffect, useRef } from 'react';
import { useSearchParams } from 'umi';
import { useHandleSearchChange } from './logic-hooks';
import { useSetPaginationParams } from './route-hook';

export const useKnowledgeBaseId = (): string => {
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id');

  return knowledgeBaseId || '';
};

export const useFetchKnowledgeBaseConfiguration = () => {
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<IKnowledge>({
    queryKey: ['fetchKnowledgeDetail'],
    initialData: {} as IKnowledge,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await kbService.get_kb_detail({
        kb_id: knowledgeBaseId,
      });
      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useFetchKnowledgeList = (
  shouldFilterListWithoutDocument: boolean = false,
): {
  list: IKnowledge[];
  loading: boolean;
} => {
  const { data, isFetching: loading } = useQuery({
    queryKey: ['fetchKnowledgeList'],
    initialData: [],
    gcTime: 0, // https://tanstack.com/query/latest/docs/framework/react/guides/caching?from=reactQueryV3
    queryFn: async () => {
      const { data } = await listDataset();
      const list = data?.data?.kbs ?? [];
      return shouldFilterListWithoutDocument
        ? list.filter((x: IKnowledge) => x.chunk_num > 0)
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

export const useInfiniteFetchKnowledgeList = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const PageSize = 30;

  const {
    data,
    error,
    fetchNextPage,
    hasNextPage,
    isFetching,
    isFetchingNextPage,
    status,
  } = useInfiniteQuery({
    queryKey: ['infiniteFetchKnowledgeList', debouncedSearchString],
    queryFn: async ({ pageParam }) => {
      const { data } = await listDataset({
        page: pageParam,
        page_size: PageSize,
        keywords: debouncedSearchString,
      });
      const list = data?.data ?? [];
      return list;
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage, pages, lastPageParam) => {
      if (lastPageParam * PageSize <= lastPage.total) {
        return lastPageParam + 1;
      }
      return undefined;
    },
  });
  return {
    data,
    loading: isFetching,
    error,
    fetchNextPage,
    hasNextPage,
    isFetching,
    isFetchingNextPage,
    status,
    handleInputChange,
    searchString,
  };
};

export const useCreateKnowledge = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['infiniteFetchKnowledgeList'],
    mutationFn: async (params: { id?: string; name: string }) => {
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
    mutationKey: ['deleteKnowledge'],
    mutationFn: async (id: string) => {
      const { data } = await kbService.rmKb({ kb_id: id });
      if (data.code === 0) {
        message.success(i18n.t(`message.deleted`));
        queryClient.invalidateQueries({
          queryKey: ['infiniteFetchKnowledgeList'],
        });
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, deleteKnowledge: mutateAsync };
};

//#region knowledge configuration

export const useUpdateKnowledge = (shouldFetchList = false) => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['saveKnowledge'],
    mutationFn: async (params: Record<string, any>) => {
      const { data = {} } = await kbService.updateKb({
        kb_id: params?.kb_id ? params?.kb_id : knowledgeBaseId,
        ...params,
      });
      if (data.code === 0) {
        message.success(i18n.t(`message.updated`));
        if (shouldFetchList) {
          queryClient.invalidateQueries({
            queryKey: ['fetchKnowledgeListByPage'],
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

//#endregion

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
      const { data } = await kbService.retrieval_test({
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
      const { data } = await kbService.retrieval_test({
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

//#region tags

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

//#endregion

export function useFetchKnowledgeGraph() {
  const knowledgeBaseId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<IKnowledgeGraph>({
    queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
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

export const useRemoveKnowledgeGraph = () => {
  const knowledgeBaseId = useKnowledgeBaseId();

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['removeKnowledgeGraph'],
    mutationFn: async () => {
      const { data } = await deleteKnowledgeGraph(knowledgeBaseId);
      if (data.code === 0) {
        message.success(i18n.t(`message.deleted`));
        queryClient.invalidateQueries({
          queryKey: ['fetchKnowledgeGraph'],
        });
      }
      return data?.code;
    },
  });

  return { data, loading, removeKnowledgeGraph: mutateAsync };
};

export const useResolveEntities = () => {
  const knowledgeBaseId = useKnowledgeBaseId();

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['resolveEntities'],
    mutationFn: async () => {
      const { data } = await resolveEntities(knowledgeBaseId);
      if (data.code === 0) {
        message.success(i18n.t(`knowledgeGraph.entityResolutionSuccess`, 'Entity resolution completed successfully'));
        queryClient.invalidateQueries({
          queryKey: ['fetchKnowledgeGraph'],
        });
      }
      return data;
    },
  });

  return { data, loading, resolveEntities: mutateAsync };
};

export const useDetectCommunities = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const [progress, setProgress] = useState(null);
  const pollingRef = useRef(null);

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['detectCommunities'],
    mutationFn: async () => {
      // Start the community detection operation
      const { data } = await detectCommunities(knowledgeBaseId);
      return data;
    },
    onSuccess: (data) => {
      if (data.code === 0) {
        message.success(i18n.t(`knowledgeGraph.communityDetectionSuccess`, 'Community detection completed successfully'));
        queryClient.invalidateQueries({
          queryKey: ['fetchKnowledgeGraph'],
        });
        
        // Clear progress after a delay
        setTimeout(() => setProgress(null), 3000);
      }
    },
    onError: () => {
      setProgress(null);
      // Clear any ongoing polling
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    },
  });

  // Function to start polling
  const startPolling = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
    }
    
    pollingRef.current = setInterval(async () => {
      try {
        const { data: progressData } = await getCommunityDetectionProgress(knowledgeBaseId);
        if (progressData.code === 0 && progressData.data) {
          setProgress(progressData.data);
          
          // If status is completed, clear progress after a delay
          if (progressData.data.current_status === 'completed') {
            setTimeout(() => {
              setProgress(null);
            }, 3000); // Clear after 3 seconds
            
            // Stop polling since operation is completed
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
          }
        } else if (progressData.code === 0 && progressData.data === null) {
          // Operation completed or not running, stop polling
          if (pollingRef.current) {
            clearInterval(pollingRef.current);
            pollingRef.current = null;
          }
        }
      } catch (error) {
        console.error('Failed to fetch progress:', error);
      }
    }, 1000); // Poll every second
  };

  // Check for ongoing operation on component mount
  useEffect(() => {
    const checkInitialProgress = async () => {
      try {
        const { data: progressData } = await getCommunityDetectionProgress(knowledgeBaseId);
        
        if (progressData.code === 0 && progressData.data) {
          setProgress(progressData.data);
          
          // If status is completed, clear progress after a delay
          if (progressData.data.current_status === 'completed') {
            setTimeout(() => {
              setProgress(null);
            }, 3000); // Clear after 3 seconds
          } else {
            // Start polling since operation is still ongoing
            startPolling();
          }
        }
      } catch (error) {
        console.error('Failed to check initial progress:', error);
      }
    };
    
    if (knowledgeBaseId) {
      checkInitialProgress();
    }
  }, [knowledgeBaseId]);

  // Start polling when mutation starts
  useEffect(() => {
    if (loading) {
      // Reset progress at start
      setProgress({
        total_communities: 0,
        processed_communities: 0,
        tokens_used: 0,
        current_status: 'starting'
      });

      // Start polling for progress
      startPolling();
    } else {
      // Stop polling when mutation completes
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    }

    return () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [loading, knowledgeBaseId]);

  return { data, loading, detectCommunities: mutateAsync, progress };
};
