import { IAgentVersionCatalogItem } from '@/interfaces/database/agent';
import agentService from '@/services/agent-service';
import { useQuery } from '@tanstack/react-query';

export const PipelineVersionKeys = {
  catalog: () => ['pipelineVersionCatalog', 'dataflow'] as const,
};

export function useFetchPipelineVersionCatalog(enabled = true) {
  const {
    data,
    isFetching: loading,
    isError,
  } = useQuery({
    queryKey: PipelineVersionKeys.catalog(),
    enabled,
    staleTime: 60_000,
    gcTime: 5 * 60_000,
    queryFn: async (): Promise<IAgentVersionCatalogItem[]> => {
      const { data } = await agentService.listAgentVersions(
        { params: { category: 'dataflow' } },
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
