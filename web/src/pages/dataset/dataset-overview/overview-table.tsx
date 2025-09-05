import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  ColumnDef,
  ColumnFiltersState,
  SortingState,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { Copy, Eye } from 'lucide-react';
import { FC, useMemo, useState } from 'react';
import FileStatusBadge from './file-status-badge';

interface DocumentLog {
  id: string;
  fileName: string;
  source: string;
  pipeline: string;
  startDate: string;
  task: string;
  status: 'Success' | 'Failed' | 'Running' | 'Pending';
}

interface FileLogsTableProps {
  data: DocumentLog[];
  pageCount: number;
  pagination: {
    current: number;
    pageSize: number;
    total: number;
  };
  setPagination: (pagination: { page: number; pageSize: number }) => void;
  loading?: boolean;
}

export const useFileLogsTableColumns = () => {
  const columns: ColumnDef<DocumentLog>[] = [
    {
      id: 'select',
      header: ({ table }) => (
        <input
          type="checkbox"
          checked={table.getIsAllRowsSelected()}
          onChange={table.getToggleAllRowsSelectedHandler()}
          className="rounded bg-gray-900 text-blue-500 focus:ring-blue-500"
        />
      ),
      cell: ({ row }) => (
        <input
          type="checkbox"
          checked={row.getIsSelected()}
          onChange={row.getToggleSelectedHandler()}
          className="rounded border-gray-600 bg-gray-900 text-blue-500 focus:ring-blue-500"
        />
      ),
    },
    {
      accessorKey: 'id',
      header: 'ID',
      cell: ({ row }) => (
        <div className="text-text-primary">{row.original.id}</div>
      ),
    },
    {
      accessorKey: 'fileName',
      header: 'File Name',
      cell: ({ row }) => (
        <div className="flex items-center gap-2 text-text-primary">
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M14 2H6a2 2 0 0 0-2 2v16h2"></path>
            <path d="M14 2v4a2 2 0 0 0 2 2h4"></path>
          </svg>
          {row.original.fileName}
        </div>
      ),
    },
    {
      accessorKey: 'source',
      header: 'Source',
      cell: ({ row }) => (
        <div className="text-text-primary">{row.original.source}</div>
      ),
    },
    {
      accessorKey: 'pipeline',
      header: 'Data Pipeline',
      cell: ({ row }) => (
        <div className="flex items-center gap-2 text-text-primary">
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <circle cx="12" cy="12" r="10"></circle>
            <path d="M12 8v4"></path>
            <path d="M12 16h.01"></path>
          </svg>
          {row.original.pipeline}
        </div>
      ),
    },
    {
      accessorKey: 'startDate',
      header: 'Start Date',
      cell: ({ row }) => (
        <div className="text-text-primary">{row.original.startDate}</div>
      ),
    },
    {
      accessorKey: 'task',
      header: 'Task',
      cell: ({ row }) => (
        <div className="text-text-primary">{row.original.task}</div>
      ),
    },
    {
      accessorKey: 'status',
      header: 'States',
      cell: ({ row }) => <FileStatusBadge status={row.original.status} />,
    },
    {
      id: 'operations',
      header: 'Operations',
      cell: ({ row }) => (
        <div className="flex justify-end space-x-2">
          <Button variant="ghost" size="sm" className="p-1">
            <Eye className="w-4 h-4" />
          </Button>
          <Button variant="ghost" size="sm" className="p-1">
            <Copy className="w-4 h-4" />
          </Button>
        </div>
      ),
    },
  ];

  return columns;
};

const FileLogsTable: FC<FileLogsTableProps> = ({
  data,
  pagination,
  setPagination,
  loading,
}) => {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [rowSelection, setRowSelection] = useState({});

  const columns = useFileLogsTableColumns();

  const currentPagination = useMemo(
    () => ({
      pageIndex: (pagination.current || 1) - 1,
      pageSize: pagination.pageSize || 10,
    }),
    [pagination],
  );

  const table = useReactTable({
    data,
    columns,
    manualPagination: true,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    onRowSelectionChange: setRowSelection,
    state: {
      sorting,
      columnFilters,
      rowSelection,
      pagination: currentPagination,
    },
    pageCount: pagination.total
      ? Math.ceil(pagination.total / pagination.pageSize)
      : 0,
  });

  return (
    <div className="w-full">
      <Table rootClassName="max-h-[calc(100vh-350px)]">
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {flexRender(
                    header.column.columnDef.header,
                    header.getContext(),
                  )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody className="relative">
          {table.getRowModel().rows.length ? (
            table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                data-state={row.getIsSelected() && 'selected'}
                className="group"
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell
                    key={cell.id}
                    className={cell.column.columnDef.meta?.cellClassName}
                  >
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={columns.length} className="h-24 text-center">
                No results.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      <div className="flex items-center justify-end py-4 absolute bottom-3 right-3">
        <div className="space-x-2">
          <RAGFlowPagination
            {...{ current: pagination.current, pageSize: pagination.pageSize }}
            total={pagination.total}
            onChange={(page, pageSize) => setPagination({ page, pageSize })}
          />
        </div>
      </div>
    </div>
  );
};

export default FileLogsTable;
