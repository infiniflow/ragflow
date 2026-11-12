import type { ReactNode } from 'react';
import { TableEmpty } from '@/components/table-skeleton';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

interface RecordTableProps {
  headers: ReactNode[];
  rows: { id: string; cells: ReactNode[] }[];
  loading: boolean;
  total: number;
  page: number;
  pageSize: number;
  onPageChange: (page: number, pageSize: number) => void;
}

export default function AdminRecordTable({
  headers,
  rows,
  loading,
  total,
  page,
  pageSize,
  onPageChange,
}: RecordTableProps) {
  return (
    <>
      <Table aria-busy={loading}>
        <TableHeader>
          <TableRow>
            {headers.map((header, index) => (
              <TableHead key={index}>{header}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.length ? (
            rows.map((row) => (
              <TableRow key={row.id}>
                {row.cells.map((cell, index) => (
                  <TableCell key={index}>{cell}</TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableEmpty columnsLength={headers.length} />
          )}
        </TableBody>
      </Table>
      <div className="flex justify-end py-4">
        <RAGFlowPagination
          total={total}
          current={page}
          pageSize={pageSize}
          onChange={onPageChange}
        />
      </div>
    </>
  );
}
