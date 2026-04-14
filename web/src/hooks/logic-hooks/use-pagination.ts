import { useCallback, useMemo, useState } from 'react';

export function useClientPagination(list: Array<any>) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const onPaginationChange = useCallback((page: number, pageSize: number) => {
    setPage(page);
    setPageSize(pageSize);
  }, []);

  const pagedList = useMemo(() => {
    return list?.slice((page - 1) * pageSize, page * pageSize);
  }, [list, page, pageSize]);

  return {
    page,
    pageSize,
    setPage,
    setPageSize,
    onPaginationChange,
    pagedList,
  };
}
