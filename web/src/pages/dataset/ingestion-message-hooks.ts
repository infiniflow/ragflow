import {
  IngestionMessagesResponse,
  IngestionMessageParams,
} from '@/interfaces/database/ingestion';
import { listIngestionMessages } from '@/services/knowledge-service';
import { useIsGoBackend } from '@/utils/backend-variant';
import { useInfiniteQuery } from '@tanstack/react-query';
import type { InfiniteData } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';

const PollIntervalMs = 5000;

export const IngestionMessageKeys = {
  messages: (datasetId: string | undefined, logId: string | undefined) =>
    ['ingestionMessages', datasetId, logId] as const,
};

export const useIngestionMessages = (
  datasetId: string | undefined,
  logId: string | undefined,
  enabled: boolean,
  params: IngestionMessageParams = { limit: 200 },
) => {
  const isGoBackend = useIsGoBackend();
  const [finishing, setFinishing] = useState(false);
  const finishingRef = useRef(false);

  const query = useInfiniteQuery<
    IngestionMessagesResponse,
    Error,
    InfiniteData<IngestionMessagesResponse, IngestionMessageParams | undefined>,
    readonly unknown[],
    IngestionMessageParams | undefined
  >({
    queryKey: IngestionMessageKeys.messages(datasetId, logId),
    enabled: enabled && isGoBackend && !!datasetId && !!logId,
    refetchInterval: (query) => {
      if (finishing) {
        return false;
      }
      return query.state.data?.pages.some((page) => page.terminal)
        ? false
        : PollIntervalMs;
    },
    initialPageParam: undefined,
    queryFn: async ({ pageParam }) => {
      const { data: res = {} } = await listIngestionMessages(
        datasetId || '',
        logId || '',
        { ...params, ...pageParam },
      );
      return res.data as IngestionMessagesResponse;
    },
    getNextPageParam: (lastPage) =>
      lastPage.has_more_after && lastPage.newest_id
        ? { after_id: lastPage.newest_id }
        : undefined,
    getPreviousPageParam: (firstPage) =>
      firstPage.has_more_before && firstPage.oldest_id
        ? { before_id: firstPage.oldest_id }
        : undefined,
  });

  const { fetchNextPage, hasNextPage, isFetchingNextPage, refetch } = query;
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage) {
      void fetchNextPage();
    }
  }, [fetchNextPage, hasNextPage, isFetchingNextPage]);

  const pages = query.data?.pages ?? [];
  const terminal = pages.some((page) => page.terminal);
  useEffect(() => {
    finishingRef.current = false;
    setFinishing(false);
  }, [datasetId, logId]);
  useEffect(() => {
    if (!terminal || finishingRef.current) {
      return;
    }
    finishingRef.current = true;
    setFinishing(true);
    const timer = setTimeout(() => {
      void refetch().finally(() => setFinishing(false));
    }, PollIntervalMs);
    return () => clearTimeout(timer);
  }, [refetch, terminal]);

  const items = Array.from(
    new Map(
      pages.flatMap((page) => page.items).map((item) => [item.id, item]),
    ).values(),
  ).sort((left, right) => left.id - right.id);
  return {
    ...query,
    data: pages.length
      ? {
          ...pages[0],
          items,
          terminal,
        }
      : undefined,
  };
};
