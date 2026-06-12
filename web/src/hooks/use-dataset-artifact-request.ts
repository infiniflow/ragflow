import message from '@/components/ui/message';
import {
  ArtifactGraphResponse,
  ArtifactListResponse,
  ArtifactPage,
  HasAnyArtifactResponse,
} from '@/interfaces/database/dataset-artifact';
import i18n from '@/locales/config';
import datasetArtifactService from '@/services/dataset-artifact-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useKnowledgeBaseId } from './use-knowledge-request';

/**
 * Query-key factory for the dataset Artifact tab. Keyed by KB id so two
 * KBs' caches stay independent; the page-detail key includes pageType +
 * slug for per-page cache granularity.
 */
export const DatasetArtifactKeys = {
  all: (kbId: string) => ['dataset_artifact', kbId] as const,
  has: (kbId: string) => ['dataset_artifact', kbId, 'has'] as const,
  list: (
    kbId: string,
    filters: { page?: number; pageSize?: number; pageType?: string } = {},
  ) => ['dataset_artifact', kbId, 'list', filters] as const,
  page: (kbId: string, pageType: string, slug: string) =>
    ['dataset_artifact', kbId, 'page', pageType, slug] as const,
  graph: (kbId: string) => ['dataset_artifact', kbId, 'graph'] as const,
};

/**
 * Existence probe used by the dataset sidebar to decide whether to show
 * the Artifact tab. Mirrors the KG pattern (`useFetchKnowledgeGraph`).
 *
 * Note on `initialData` + no `staleTime`: React Query treats `initialData`
 * as a fresh cache hit, so combining it with any non-zero `staleTime`
 * suppresses the network fetch on mount/refresh. We rely on the default
 * `staleTime: 0` here so the probe always re-runs and the tab visibility
 * is correct after a hard refresh.
 */
export function useHasAnyArtifact() {
  const kbId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<HasAnyArtifactResponse>({
    queryKey: DatasetArtifactKeys.has(kbId),
    initialData: { has: false },
    enabled: !!kbId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetArtifactService.hasAny({ datasetId: kbId });
      return data?.data ?? { has: false };
    },
  });

  return { data, loading };
}

export function useListDatasetArtifacts(
  filters: {
    page?: number;
    pageSize?: number;
    pageType?: string;
  } = {},
) {
  const kbId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<ArtifactListResponse>({
    queryKey: DatasetArtifactKeys.list(kbId, filters),
    initialData: { total: 0, items: [] },
    enabled: !!kbId,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetArtifactService.list({
        datasetId: kbId,
        page: filters.page,
        page_size: filters.pageSize,
        page_type: filters.pageType,
      });
      return data?.data ?? { total: 0, items: [] };
    },
  });

  return { data: data ?? { total: 0, items: [] }, loading };
}

export function useFetchDatasetArtifactPage(
  pageType: string | undefined,
  slug: string | undefined,
) {
  const kbId = useKnowledgeBaseId();
  const enabled = !!kbId && !!pageType && !!slug;

  const { data, isFetching: loading } = useQuery<ArtifactPage | null>({
    queryKey: DatasetArtifactKeys.page(kbId, pageType ?? '', slug ?? ''),
    initialData: null,
    enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetArtifactService.getPage({
        datasetId: kbId,
        pageType: pageType!,
        slug: slug!,
      });
      return data?.data ?? null;
    },
  });

  return { data, loading };
}

/**
 * Wipe all artifact rows for this KB. On success, every cached query under
 * the KB's artifact namespace is invalidated so the sidebar tab probe
 * (``useHasAnyArtifact``), the list view, and any open page detail all
 * refetch and reflect the empty state.
 */
export function useClearDatasetArtifacts() {
  const kbId = useKnowledgeBaseId();
  const queryClient = useQueryClient();

  const { isPending: loading, mutateAsync } = useMutation({
    mutationKey: ['clearDatasetArtifacts', kbId],
    mutationFn: async () => {
      const { data = {} } = await datasetArtifactService.clear({
        datasetId: kbId,
      });
      if (data.code === 0) {
        message.success(i18n.t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: DatasetArtifactKeys.all(kbId),
        });
      }
      return data.code;
    },
  });

  return { loading, clearArtifacts: mutateAsync };
}

const EMPTY_GRAPH: ArtifactGraphResponse = {
  entities: [],
  relations: [],
};

/**
 * Fetch the REDUCE-phase graph payload (entities / concepts / claims /
 * relations). Enabled only when ``enabled`` is true so the graph view
 * opens its own query lazily — the markdown viewer doesn't pay for it.
 */
export function useFetchDatasetArtifactGraph(enabled: boolean) {
  const kbId = useKnowledgeBaseId();

  const { data, isFetching: loading } = useQuery<ArtifactGraphResponse>({
    queryKey: DatasetArtifactKeys.graph(kbId),
    initialData: EMPTY_GRAPH,
    enabled: !!kbId && enabled,
    gcTime: 0,
    queryFn: async () => {
      const { data } = await datasetArtifactService.getGraph({
        datasetId: kbId,
      });
      return (data?.data as ArtifactGraphResponse) ?? EMPTY_GRAPH;
    },
  });

  return { data: data ?? EMPTY_GRAPH, loading };
}
