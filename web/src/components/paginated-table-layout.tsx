import { Table } from '@/components/ui/table';
import { PropsWithChildren, ReactNode } from 'react';

type PaginatedTableLayoutProps = PropsWithChildren<{
  pagination: ReactNode;
  footerLeading?: ReactNode;
  tableClassName?: string;
}>;

export function PaginatedTableLayout({
  children,
  pagination,
  footerLeading,
  tableClassName,
}: PaginatedTableLayoutProps) {
  return (
    <div className="w-full flex-1 min-h-0 flex flex-col">
      <Table rootClassName="flex-1 min-h-0 mb-4" className={tableClassName}>
        {children}
      </Table>
      <div className="mt-auto shrink-0 flex items-center justify-end gap-4 pb-5">
        {footerLeading && <div className="min-w-0 flex-1">{footerLeading}</div>}
        <div className="ml-auto shrink-0">{pagination}</div>
      </div>
    </div>
  );
}
