import {
  IngestionMessagesResponse,
  IngestionMessageParams,
} from '@/interfaces/database/ingestion';
import { listIngestionMessages } from '@/services/knowledge-service';
import { useIsGoBackend } from '@/utils/backend-variant';
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import type { InfiniteData } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';

const PollIntervalMs = 5000;

// Module-level constant: an inline default object would get a new identity on
// every render and, being a polling-effect dependency, would reset the
// interval before it ever fires.
const DefaultParams: IngestionMessageParams = { limit: 200 };

export const IngestionMessageKeys = {
  messages: (datasetId: string | undefined, logId: string | undefined) =>
    ['ingestionMessages', datasetId, logId] as const,
};

export const useIngestionMessages = (
  datasetId: string | undefined,
  logId: string | undefined,
  enabled: boolean,
  params: IngestionMessageParams = DefaultParams,
) => {
  const queryClient = useQueryClient();
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

  const { fetchNextPage, hasNextPage, isFetchingNextPage } = query;
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage) {
      void fetchNextPage();
    }
  }, [fetchNextPage, hasNextPage, isFetchingNextPage]);

  const pages = query.data?.pages ?? [];
  const terminal = pages.some((page) => page.terminal);

  const items = Array.from(
    new Map(
      pages.flatMap((page) => page.items).map((item) => [item.id, item]),
    ).values(),
  ).sort((left, right) => left.id - right.id);

  const newestId = items.length > 0 ? items[items.length - 1].id : undefined;
  const newestIdRef = useRef(newestId);
  newestIdRef.current = newestId;

  useEffect(() => {
    if (
      !enabled ||
      !isGoBackend ||
      !datasetId ||
      !logId ||
      terminal ||
      finishing
    ) {
      return;
    }
    const interval = setInterval(async () => {
      if (isFetchingNextPage || hasNextPage) {
        return;
      }
      try {
        const afterId = newestIdRef.current;
        const { data: res = {} } = await listIngestionMessages(
          datasetId,
          logId,
          {
            ...params,
            ...(afterId ? { after_id: afterId } : {}),
          },
        );
        const nextResponse = res.data as IngestionMessagesResponse | undefined;
        if (!nextResponse) {
          return;
        }
        if (nextResponse.items?.length > 0) {
          queryClient.setQueryData<
            InfiniteData<
              IngestionMessagesResponse,
              IngestionMessageParams | undefined
            >
          >(IngestionMessageKeys.messages(datasetId, logId), (oldData) => {
            if (!oldData) return oldData;
            return {
              ...oldData,
              pages: [...oldData.pages, nextResponse],
              pageParams: [
                ...oldData.pageParams,
                afterId ? { after_id: afterId } : undefined,
              ],
            };
          });
        } else if (nextResponse.terminal) {
          queryClient.setQueryData<
            InfiniteData<
              IngestionMessagesResponse,
              IngestionMessageParams | undefined
            >
          >(IngestionMessageKeys.messages(datasetId, logId), (oldData) => {
            if (!oldData || oldData.pages.length === 0) return oldData;
            const lastIndex = oldData.pages.length - 1;
            const lastPage = oldData.pages[lastIndex];
            if (lastPage.terminal) return oldData;
            const nextPages = [...oldData.pages];
            nextPages[lastIndex] = { ...lastPage, terminal: true };
            return { ...oldData, pages: nextPages };
          });
        }
      } catch {
        // ignore polling error
      }
    }, PollIntervalMs);
    return () => clearInterval(interval);
  }, [
    enabled,
    isGoBackend,
    datasetId,
    logId,
    terminal,
    finishing,
    hasNextPage,
    isFetchingNextPage,
    params,
    queryClient,
  ]);

  useEffect(() => {
    finishingRef.current = false;
    setFinishing(false);
  }, [datasetId, logId]);

  useEffect(() => {
    if (
      !terminal ||
      finishingRef.current ||
      !enabled ||
      !isGoBackend ||
      !datasetId ||
      !logId
    ) {
      return;
    }
    finishingRef.current = true;
    setFinishing(true);
    const timer = setTimeout(async () => {
      try {
        const afterId = newestIdRef.current;
        const { data: res = {} } = await listIngestionMessages(
          datasetId,
          logId,
          {
            ...params,
            ...(afterId ? { after_id: afterId } : {}),
          },
        );
        const nextResponse = res.data as IngestionMessagesResponse | undefined;
        if (nextResponse?.items?.length) {
          queryClient.setQueryData<
            InfiniteData<
              IngestionMessagesResponse,
              IngestionMessageParams | undefined
            >
          >(IngestionMessageKeys.messages(datasetId, logId), (oldData) => {
            if (!oldData) return oldData;
            return {
              ...oldData,
              pages: [...oldData.pages, nextResponse],
              pageParams: [
                ...oldData.pageParams,
                afterId ? { after_id: afterId } : undefined,
              ],
            };
          });
        }
      } finally {
        setFinishing(false);
      }
    }, PollIntervalMs);
    return () => clearTimeout(timer);
  }, [datasetId, enabled, isGoBackend, logId, params, queryClient, terminal]);

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
