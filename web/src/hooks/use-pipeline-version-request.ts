import { IAgentVersionCatalogItem } from '@/interfaces/database/agent';
import { ModelResourceAccessUserIdContext } from '@/hooks/use-llm-request';
import agentService from '@/services/agent-service';
import { useQuery } from '@tanstack/react-query';
import { useContext } from 'react';

export const PipelineVersionKeys = {
  catalog: (accessUserId?: string) =>
    accessUserId
      ? (['pipelineVersionCatalog', 'dataflow', accessUserId] as const)
      : (['pipelineVersionCatalog', 'dataflow'] as const),
};

export function useFetchPipelineVersionCatalog(enabled = true) {
  const accessUserId = useContext(ModelResourceAccessUserIdContext);
  const {
    data,
    isFetching: loading,
    isError,
  } = useQuery({
    queryKey: PipelineVersionKeys.catalog(accessUserId),
    enabled,
    staleTime: 60_000,
    gcTime: 5 * 60_000,
    queryFn: async (): Promise<IAgentVersionCatalogItem[]> => {
      const { data } = await agentService.listAgentVersions(
        {
          params: {
            category: 'dataflow',
            ...(accessUserId ? { access_user_id: accessUserId } : {}),
          },
        },
        true,
      );
      if (data?.code !== 0) throw new Error(data?.message);
      return data.data ?? [];
    },
  });

  return { data, loading, isError };
}

export function useFetchPipelineVersionList(pipelineId?: string) {
  const { data, loading, isError } =
    useFetchPipelineVersionCatalog(!!pipelineId);
  return {
    data:
      pipelineId && data
        ? (data.find((pipeline) => pipeline.id === pipelineId)?.versions ?? [])
        : undefined,
    loading,
    isError,
  };
}
