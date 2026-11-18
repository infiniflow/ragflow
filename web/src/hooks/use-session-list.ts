import { useInfiniteQuery } from '@tanstack/react-query';
import { uniqBy } from 'lodash';
import { UIEvent, useCallback, useMemo } from 'react';

type SessionPage<T> = {
  items: T[];
  total: number;
};

export function useSessionList<T extends { id: string }>({
  queryKey,
  enabled,
  fetchPage,
}: {
  queryKey: readonly unknown[];
  enabled: boolean;
  fetchPage: (params: {
    page: number;
    page_size: number;
  }) => Promise<SessionPage<T>>;
}) {
  const {
    data,
    isFetching,
    isFetchingNextPage,
    hasNextPage,
    fetchNextPage,
    refetch,
  } = useInfiniteQuery({
    queryKey,
    enabled,
    gcTime: 0,
    refetchOnWindowFocus: false,
    initialPageParam: 0,
    queryFn: ({ pageParam: offset }) => {
      // Keep offsets continuous when switching from 30 initial rows to 10-row pages.
      const pageSize = offset === 0 ? 30 : 10;
      return fetchPage({ page: offset / pageSize + 1, page_size: pageSize });
    },
    getNextPageParam: (lastPage, pages, offset) => {
      const pageSize = offset === 0 ? 30 : 10;
      const loadedCount = pages.reduce(
        (count, page) => count + page.items.length,
        0,
      );
      if (lastPage.items.length < pageSize || loadedCount >= lastPage.total) {
        return undefined;
      }
      return offset + pageSize;
    },
  });

  const sessions = useMemo(
    () => uniqBy(data?.pages.flatMap((page) => page.items) ?? [], 'id'),
    [data],
  );

  const handleScroll = useCallback(
    (event: UIEvent<HTMLDivElement>) => {
      const { scrollTop, scrollHeight, clientHeight } = event.currentTarget;
      if (
        scrollHeight - scrollTop - clientHeight <= 50 &&
        hasNextPage &&
        !isFetching
      ) {
        void fetchNextPage({ cancelRefetch: false });
      }
    },
    [fetchNextPage, hasNextPage, isFetching],
  );

  return {
    data: sessions,
    total: data?.pages.at(-1)?.total ?? 0,
    loading: isFetching,
    loadingMore: isFetchingNextPage,
    hasMore: !!hasNextPage,
    handleScroll,
    refetch,
  };
}
