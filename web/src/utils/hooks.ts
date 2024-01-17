import { useState } from 'react';

export const usePagination = function (defaultPage: number, defaultPageSize: number, total: number) {
  const [page = 1, setPage] = useState(defaultPage);
  const [pageSize = 10, setPageSize] = useState(defaultPageSize);
  return {
    page,
    pageSize,
    count: total,
    setPage,
    setPageSize,
    nextPage: () => setPage(page + 1)
  };
};
