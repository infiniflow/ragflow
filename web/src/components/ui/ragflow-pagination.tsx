import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination';
import { RAGFlowSelect, RAGFlowSelectOptionType } from '@/components/ui/select';
import { cn } from '@/lib/utils';
import { useCallback, useEffect, useMemo, useState } from 'react';

export type RAGFlowPaginationType = {
  showQuickJumper?: boolean;
  onChange?(page: number, pageSize: number): void;
  total?: number;
  current?: number;
  pageSize?: number;
  showSizeChanger?: boolean;
};

export function RAGFlowPagination({
  current = 1,
  pageSize = 10,
  total = 0,
  onChange,
  showSizeChanger = true,
}: RAGFlowPaginationType) {
  const [currentPage, setCurrentPage] = useState(1);
  const [currentPageSize, setCurrentPageSize] = useState('10');

  const sizeChangerOptions: RAGFlowSelectOptionType[] = useMemo(() => {
    return [10, 20, 50, 100].map((x) => ({
      label: <span>{x} / page</span>,
      value: x.toString(),
    }));
  }, []);

  const pages = useMemo(() => {
    const num = Math.ceil(total / pageSize);
    return new Array(num).fill(0).map((_, idx) => idx + 1);
  }, [pageSize, total]);

  const changePage = useCallback(
    (page: number) => {
      onChange?.(page, Number(currentPageSize));
    },
    [currentPageSize, onChange],
  );

  const handlePreviousPageChange = useCallback(() => {
    setCurrentPage((page) => {
      const previousPage = page - 1;
      if (previousPage > 0) {
        changePage(previousPage);
        return previousPage;
      }
      changePage(page);
      return page;
    });
  }, [changePage]);

  const handlePageChange = useCallback(
    (page: number) => () => {
      changePage(page);
      setCurrentPage(page);
    },
    [changePage],
  );

  const handleNextPageChange = useCallback(() => {
    setCurrentPage((page) => {
      const nextPage = page + 1;
      if (nextPage <= pages.length) {
        changePage(nextPage);
        return nextPage;
      }
      changePage(page);
      return page;
    });
  }, [changePage, pages.length]);

  const handlePageSizeChange = useCallback(
    (size: string) => {
      onChange?.(currentPage, Number(size));
      setCurrentPageSize(size);
    },
    [currentPage, onChange],
  );

  useEffect(() => {
    setCurrentPage(current);
  }, [current]);

  useEffect(() => {
    setCurrentPageSize(pageSize.toString());
  }, [pageSize]);

  return (
    <section className="flex items-center justify-end text-text-sub-title-invert ">
      <span className="mr-4">Total {total}</span>
      <Pagination className="w-auto mx-0 mr-4">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious onClick={handlePreviousPageChange} />
          </PaginationItem>
          {pages.map((x) => (
            <PaginationItem
              key={x}
              className={cn({
                ['bg-background-header-bar rounded-md text-text-title']:
                  currentPage === x,
              })}
            >
              <PaginationLink onClick={handlePageChange(x)} className="size-8">
                {x}
              </PaginationLink>
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
      {showSizeChanger && (
        <RAGFlowSelect
          options={sizeChangerOptions}
          value={currentPageSize}
          onChange={handlePageSizeChange}
          triggerClassName="bg-background-header-bar"
        ></RAGFlowSelect>
      )}
    </section>
  );
}
