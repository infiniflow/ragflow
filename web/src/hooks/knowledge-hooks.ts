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
  getEntityResolutionProgress,
  checkDocumentParsing,
  extractEntities,
  buildGraph,
  getExtractionProgress,
  getBuildProgress,
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

// Helper functions for progress dismissal storage
const getDismissalKey = (knowledgeBaseId: string, progressType: string) => 
  `progress_dismissed_${knowledgeBaseId}_${progressType}`;

const isProgressDismissed = (knowledgeBaseId: string, progressType: string): boolean => {
  const key = getDismissalKey(knowledgeBaseId, progressType);
  return localStorage.getItem(key) === 'true';
};

const setProgressDismissed = (knowledgeBaseId: string, progressType: string): void => {
  const key = getDismissalKey(knowledgeBaseId, progressType);
  localStorage.setItem(key, 'true');
};

const clearProgressDismissal = (knowledgeBaseId: string, progressType: string): void => {
  const key = getDismissalKey(knowledgeBaseId, progressType);
  localStorage.removeItem(key);
};

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

  const { data, isFetching: loading, refetch } = useQuery<IKnowledgeGraph>({
    queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
    initialData: { graph: {}, mind_map: {} } as IKnowledgeGraph,
    enabled: !!knowledgeBaseId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await getKnowledgeGraph(knowledgeBaseId);
      return data?.data;
    },
  });

  return { data, loading, refetch };
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
          queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
        });
      }
      return data?.code;
    },
  });

  return { data, loading, removeKnowledgeGraph: mutateAsync };
};

export const useResolveEntities = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const [progress, setProgress] = useState(null);
  const pollingRef = useRef(null);

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['resolveEntities'],
    mutationFn: async () => {
      // Start the entity resolution operation
      const { data } = await resolveEntities(knowledgeBaseId);
      return data;
    },
    onSuccess: (data) => {
      if (data.code === 0) {
        message.success(i18n.t(`knowledgeGraph.entityResolutionSuccess`, 'Entity resolution completed successfully'));
        queryClient.invalidateQueries({
          queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
        });
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
        const { data: progressData } = await getEntityResolutionProgress(knowledgeBaseId);
        if (progressData.code === 0 && progressData.data) {
          // Check if user has dismissed this completed progress
          if (progressData.data.current_status === 'completed' && isProgressDismissed(knowledgeBaseId, 'resolution')) {
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            return; // Don't show dismissed completed progress
          }
          
          setProgress(progressData.data);
          
          // If status is completed, stop polling since operation is completed
          if (progressData.data.current_status === 'completed') {
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            // Invalidate the knowledge graph query to refresh data
            queryClient.invalidateQueries({
              queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
            });
          }
        } else if (progressData.code === 0 && progressData.data === null) {
          // Operation completed or not running, stop polling
          if (pollingRef.current) {
            clearInterval(pollingRef.current);
            pollingRef.current = null;
          }
        }
      } catch (error) {
        console.error('Failed to fetch entity resolution progress:', error);
      }
    }, 3000); // Poll every 3 seconds
  };

  // Check for ongoing operation on component mount
  useEffect(() => {
    const checkInitialProgress = async () => {
      try {
        const { data: progressData } = await getEntityResolutionProgress(knowledgeBaseId);
        console.log('Entity resolution initial progress check:', progressData);
        
        if (progressData.code === 0 && progressData.data) {
          const isDismissed = isProgressDismissed(knowledgeBaseId, 'resolution');
          console.log('Entity resolution progress status:', progressData.data.current_status, 'isDismissed:', isDismissed);
          
          // Check if user has dismissed this completed progress
          if (progressData.data.current_status === 'completed' && isDismissed) {
            console.log('Entity resolution progress dismissed, not showing');
            return; // Don't show dismissed completed progress
          }
          
          console.log('Setting entity resolution progress:', progressData.data);
          setProgress(progressData.data);
          
          // If status is completed, don't start polling
          if (progressData.data.current_status !== 'completed') {
            // Start polling since operation is still ongoing
            startPolling();
          }
        } else {
          console.log('No entity resolution progress data available');
        }
      } catch (error) {
        console.error('Failed to check initial entity resolution progress:', error);
      }
    };
    
    if (knowledgeBaseId) {
      checkInitialProgress();
    }
  }, [knowledgeBaseId]);

  // Start polling when mutation starts
  useEffect(() => {
    if (loading) {
      // Clear dismissal when starting new resolution
      clearProgressDismissal(knowledgeBaseId, 'resolution');
      // Reset progress at start
      setProgress({
        total_pairs: 0,
        processed_pairs: 0,
        remaining_pairs: 0,
        current_status: 'starting'
      });

      console.log('Starting entity resolution - beginning polling...');
      // Start polling for progress
      startPolling();
    }
    // Don't clear polling when loading becomes false - let the polling continue until completion
  }, [loading]);
  
  // Cleanup polling when component unmounts or knowledgeBaseId changes
  useEffect(() => {
    return () => {
      console.log('useEffect cleanup (knowledgeBaseId) - clearing entity resolution polling');
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [knowledgeBaseId]);

  return { 
    data, 
    loading, 
    resolveEntities: mutateAsync, 
    progress, 
    clearProgress: () => {
      setProgress(null);
      setProgressDismissed(knowledgeBaseId, 'resolution');
    }
  };
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
          queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
        });
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
          // Check if user has dismissed this completed progress
          if (progressData.data.current_status === 'completed' && isProgressDismissed(knowledgeBaseId, 'communities')) {
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            return; // Don't show dismissed completed progress
          }
          
          setProgress(progressData.data);
          
          // If status is completed, stop polling since operation is completed
          if (progressData.data.current_status === 'completed') {
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            // Invalidate the knowledge graph query to refresh data
            queryClient.invalidateQueries({
              queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
            });
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
    }, 3000); // Poll every 3 seconds
  };

  // Check for ongoing operation on component mount
  useEffect(() => {
    const checkInitialProgress = async () => {
      try {
        const { data: progressData } = await getCommunityDetectionProgress(knowledgeBaseId);
        
        if (progressData.code === 0 && progressData.data) {
          // Check if user has dismissed this completed progress
          if (progressData.data.current_status === 'completed' && isProgressDismissed(knowledgeBaseId, 'communities')) {
            return; // Don't show dismissed completed progress
          }
          
          setProgress(progressData.data);
          
          // If status is completed, don't start polling
          if (progressData.data.current_status !== 'completed') {
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
      // Clear dismissal when starting new community detection
      clearProgressDismissal(knowledgeBaseId, 'communities');
      // Reset progress at start
      setProgress({
        total_communities: 0,
        processed_communities: 0,
        tokens_used: 0,
        current_status: 'starting'
      });

      console.log('Starting community detection - beginning polling...');
      // Start polling for progress
      startPolling();
    }
    // Don't clear polling when loading becomes false - let the polling continue until completion
  }, [loading]);
  
  // Cleanup polling when component unmounts or knowledgeBaseId changes
  useEffect(() => {
    return () => {
      console.log('useEffect cleanup (knowledgeBaseId) - clearing community detection polling');
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [knowledgeBaseId]);

  return { 
    data, 
    loading, 
    detectCommunities: mutateAsync, 
    progress, 
    clearProgress: () => {
      setProgress(null);
      setProgressDismissed(knowledgeBaseId, 'communities');
    }
  };
};

export const useCheckDocumentParsing = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const [isParsing, setIsParsing] = useState(false);
  const pollingRef = useRef(null);

  // Function to check parsing status
  const checkParsing = async () => {
    try {
      const { data } = await checkDocumentParsing(knowledgeBaseId);
      if (data.code === 0) {
        setIsParsing(data.data.is_parsing);
      }
    } catch (error) {
      console.error('Failed to check document parsing status:', error);
    }
  };

  // Start polling
  const startPolling = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
    }
    
    pollingRef.current = setInterval(checkParsing, 5000); // Poll every 5 seconds
  };

  // Stop polling  
  const stopPolling = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
      pollingRef.current = null;
    }
  };

  // Effect to start polling when knowledge base ID changes
  useEffect(() => {
    if (knowledgeBaseId) {
      checkParsing(); // Check immediately
      startPolling(); // Start polling
    }

    return () => {
      stopPolling();
    };
  }, [knowledgeBaseId]);

  return { isParsing, checkParsing, startPolling, stopPolling };
};

export const useExtractEntities = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const [progress, setProgress] = useState(null);
  const pollingRef = useRef(null);

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['extractEntities'],
    mutationFn: async () => {
      const { data } = await extractEntities(knowledgeBaseId);
      return data;
    },
    onSuccess: (data) => {
      if (data.code === 0) {
        message.success(i18n.t(`knowledgeGraph.entityExtractionSuccess`, 'Entity extraction completed successfully'));
        queryClient.invalidateQueries({
          queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
        });
      }
    },
    onError: () => {
      console.log('Mutation error - clearing polling');
      setProgress(null);
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    },
  });

  const startPolling = () => {
    if (pollingRef.current) {
      console.log('Clearing existing polling interval');
      clearInterval(pollingRef.current);
    }
    
    console.log('Starting entity extraction polling...');
    pollingRef.current = setInterval(async () => {
      try {
        console.log('Polling entity extraction progress...');
        const { data: progressData } = await getExtractionProgress(knowledgeBaseId);
        console.log('Progress data received:', progressData);
        
        if (progressData.code === 0 && progressData.data) {
          // Check if user has dismissed this completed progress
          if (progressData.data.current_status === 'completed' && isProgressDismissed(knowledgeBaseId, 'extraction')) {
            console.log('Progress completed but dismissed, stopping polling');
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            return; // Don't show dismissed completed progress
          }
          
          console.log('Setting progress:', progressData.data);
          setProgress(progressData.data);
          
          if (progressData.data.current_status === 'completed') {
            console.log('Extraction completed, stopping polling');
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            // Invalidate the knowledge graph query to refresh data
            queryClient.invalidateQueries({
              queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
            });
          }
        } else if (progressData.code === 0 && progressData.data === null) {
          console.log('No progress data, stopping polling');
          if (pollingRef.current) {
            clearInterval(pollingRef.current);
            pollingRef.current = null;
          }
        }
      } catch (error) {
        console.error('Failed to fetch entity extraction progress:', error);
      }
    }, 3000); // Poll every 3 seconds
  };

  useEffect(() => {
    const checkInitialProgress = async () => {
      try {
        const { data: progressData } = await getExtractionProgress(knowledgeBaseId);
        
        if (progressData.code === 0 && progressData.data) {
          // Check if user has dismissed this completed progress
          if (progressData.data.current_status === 'completed' && isProgressDismissed(knowledgeBaseId, 'extraction')) {
            return; // Don't show dismissed completed progress
          }
          
          setProgress(progressData.data);
          
          if (progressData.data.current_status !== 'completed') {
            startPolling();
          }
        }
      } catch (error) {
        console.error('Failed to check initial entity extraction progress:', error);
      }
    };
    
    if (knowledgeBaseId) {
      checkInitialProgress();
    }
  }, [knowledgeBaseId]);

  useEffect(() => {
    console.log('useEffect [loading] - loading:', loading, 'knowledgeBaseId:', knowledgeBaseId);
    if (loading) {
      // Clear dismissal when starting new extraction
      clearProgressDismissal(knowledgeBaseId, 'extraction');
      setProgress({
        total_documents: 0,
        processed_documents: 0,
        entities_found: 0,
        current_status: 'starting'
      });

      console.log('Starting extraction - beginning polling...');
      startPolling();
    }
    // Don't clear polling when loading becomes false - let the polling continue until completion
  }, [loading]);
  
  // Cleanup polling when component unmounts or knowledgeBaseId changes
  useEffect(() => {
    return () => {
      console.log('useEffect cleanup (knowledgeBaseId) - clearing polling');
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [knowledgeBaseId]);

  return { 
    data, 
    loading, 
    extractEntities: mutateAsync, 
    progress, 
    clearProgress: () => {
      setProgress(null);
      setProgressDismissed(knowledgeBaseId, 'extraction');
    }
  };
};

export const useBuildGraph = () => {
  const knowledgeBaseId = useKnowledgeBaseId();
  const [progress, setProgress] = useState(null);
  const pollingRef = useRef(null);

  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: ['buildGraph'],
    mutationFn: async () => {
      const { data } = await buildGraph(knowledgeBaseId);
      return data;
    },
    onSuccess: (data) => {
      if (data.code === 0) {
        message.success(i18n.t(`knowledgeGraph.graphBuildSuccess`, 'Graph building completed successfully'));
        queryClient.invalidateQueries({
          queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
        });
      }
    },
    onError: () => {
      setProgress(null);
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    },
  });

  const startPolling = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
    }
    
    pollingRef.current = setInterval(async () => {
      try {
        const { data: progressData } = await getBuildProgress(knowledgeBaseId);
        if (progressData.code === 0 && progressData.data) {
          // Check if user has dismissed this completed progress
          if (progressData.data.current_status === 'completed' && isProgressDismissed(knowledgeBaseId, 'build')) {
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            return; // Don't show dismissed completed progress
          }
          
          setProgress(progressData.data);
          
          if (progressData.data.current_status === 'completed') {
            if (pollingRef.current) {
              clearInterval(pollingRef.current);
              pollingRef.current = null;
            }
            // Invalidate the knowledge graph query to refresh data
            queryClient.invalidateQueries({
              queryKey: ['fetchKnowledgeGraph', knowledgeBaseId],
            });
          }
        } else if (progressData.code === 0 && progressData.data === null) {
          if (pollingRef.current) {
            clearInterval(pollingRef.current);
            pollingRef.current = null;
          }
        }
      } catch (error) {
        console.error('Failed to fetch graph build progress:', error);
      }
    }, 3000); // Poll every 3 seconds
  };

  useEffect(() => {
    const checkInitialProgress = async () => {
      try {
        const { data: progressData } = await getBuildProgress(knowledgeBaseId);
        
        if (progressData.code === 0 && progressData.data) {
          // Check if user has dismissed this completed progress
          if (progressData.data.current_status === 'completed' && isProgressDismissed(knowledgeBaseId, 'build')) {
            return; // Don't show dismissed completed progress
          }
          
          setProgress(progressData.data);
          
          if (progressData.data.current_status !== 'completed') {
            startPolling();
          }
        }
      } catch (error) {
        console.error('Failed to check initial graph build progress:', error);
      }
    };
    
    if (knowledgeBaseId) {
      checkInitialProgress();
    }
  }, [knowledgeBaseId]);

  useEffect(() => {
    if (loading) {
      // Clear dismissal when starting new graph build
      clearProgressDismissal(knowledgeBaseId, 'build');
      setProgress({
        total_entities: 0,
        processed_entities: 0,
        relationships_created: 0,
        current_status: 'starting'
      });

      console.log('Starting graph building - beginning polling...');
      startPolling();
    }
    // Don't clear polling when loading becomes false - let the polling continue until completion
  }, [loading]);
  
  // Cleanup polling when component unmounts or knowledgeBaseId changes
  useEffect(() => {
    return () => {
      console.log('useEffect cleanup (knowledgeBaseId) - clearing graph building polling');
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [knowledgeBaseId]);

  return { 
    data, 
    loading, 
    buildGraph: mutateAsync, 
    progress, 
    clearProgress: () => {
      setProgress(null);
      setProgressDismissed(knowledgeBaseId, 'build');
    }
  };
};
