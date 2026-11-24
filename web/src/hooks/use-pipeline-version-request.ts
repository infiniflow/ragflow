import { IAgentVersionCatalogItem } from '@/interfaces/database/agent';
import { ModelResourceAccessUserIdContext } from '@/hooks/use-llm-request';
import agentService from '@/services/agent-service';
import { useQuery } from '@tanstack/react-query';
import { useContext } from 'react';

export const PipelineVersionKeys = {
  catalog: (accessUserId?: string | null) =>
    accessUserId
      ? (['pipelineVersionCatalog', 'dataflow', accessUserId] as const)
      : (['pipelineVersionCatalog', 'dataflow'] as const),
};

export function useFetchPipelineVersionCatalog(
  enabled = true,
  requestedAccessUserId?: string,
) {
  const contextAccessUserId = useContext(ModelResourceAccessUserIdContext);
  const accessUserId = requestedAccessUserId ?? contextAccessUserId;
  const {
    data,
    isFetching: loading,
    isError,
  } = useQuery({
    queryKey: PipelineVersionKeys.catalog(accessUserId),
    enabled: enabled && accessUserId !== null,
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

  return { data, loading: loading || accessUserId === null, isError };
}

export function useFetchPipelineVersionList(
  pipelineId?: string,
  accessUserId?: string,
) {
  const { data, loading, isError } = useFetchPipelineVersionCatalog(
    !!pipelineId,
    accessUserId,
  );
  return {
    data:
      pipelineId && data
        ? (data.find((pipeline) => pipeline.id === pipelineId)?.versions ?? [])
        : undefined,
    loading,
    isError,
  };
}
