import { Table } from '@/components/ui/table';
import { PropsWithChildren, ReactNode } from 'react';

type DatasetTableLayoutProps = PropsWithChildren<{
  pagination: ReactNode;
  tableClassName?: string;
}>;

export function DatasetTableLayout({
  children,
  pagination,
  tableClassName,
}: DatasetTableLayoutProps) {
  return (
    <div className="w-full flex-1 min-h-0 flex flex-col">
      <Table rootClassName="flex-1 min-h-0 mb-4" className={tableClassName}>
        {children}
      </Table>
      <div className="mt-auto shrink-0 flex items-center justify-end pb-5">
        {pagination}
      </div>
    </div>
  );
}
