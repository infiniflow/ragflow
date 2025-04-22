import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination';
import { cn } from '@/lib/utils';
import { useCallback, useEffect, useMemo, useState } from 'react';

export type DatasetsPaginationType = {
  showQuickJumper?: boolean;
  onChange?(page: number, pageSize?: number): void;
  total?: number;
  current?: number;
  pageSize?: number;
};

export function DatasetsPagination({
  current = 1,
  pageSize = 10,
  total = 0,
  onChange,
}: DatasetsPaginationType) {
  const [currentPage, setCurrentPage] = useState(1);

  const pages = useMemo(() => {
    const num = Math.ceil(total / pageSize);
    console.log('🚀 ~ pages ~ num:', num);
    return new Array(num).fill(0).map((_, idx) => idx + 1);
  }, [pageSize, total]);

  const handlePreviousPageChange = useCallback(() => {
    setCurrentPage((page) => {
      const previousPage = page - 1;
      if (previousPage > 0) {
        return previousPage;
      }
      return page;
    });
  }, []);

  const handlePageChange = useCallback(
    (page: number) => () => {
      setCurrentPage(page);
    },
    [],
  );

  const handleNextPageChange = useCallback(() => {
    setCurrentPage((page) => {
      const nextPage = page + 1;
      if (nextPage <= pages.length) {
        return nextPage;
      }
      return page;
    });
  }, [pages.length]);

  useEffect(() => {
    setCurrentPage(current);
  }, [current]);

  useEffect(() => {
    onChange?.(currentPage);
  }, [currentPage, onChange]);

  return (
    <section className="flex  items-center justify-end">
      <span className="mr-4">Total {total}</span>
      <Pagination className="w-auto mx-0">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious onClick={handlePreviousPageChange} />
          </PaginationItem>
          {pages.map((x) => (
            <PaginationItem
              key={x}
              className={cn({ ['bg-red-500']: currentPage === x })}
            >
              <PaginationLink onClick={handlePageChange(x)}>{x}</PaginationLink>
            </PaginationItem>
          ))}
          <PaginationItem>
            <PaginationEllipsis />
          </PaginationItem>
          <PaginationItem>
            <PaginationNext onClick={handleNextPageChange} />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </section>
  );
}
