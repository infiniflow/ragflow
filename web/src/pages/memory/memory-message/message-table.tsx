import {
  ColumnDef,
  ColumnFiltersState,
  SortingState,
  VisibilityState,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import * as React from 'react';

import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Pagination } from '@/interfaces/common';
import { t } from 'i18next';
import { pick } from 'lodash';
import { Eraser, TextSelect } from 'lucide-react';
import { useMemo } from 'react';
import { IMessageInfo } from './interface';

export type MemoryTableProps = {
  messages: Array<IMessageInfo>;
  total: number;
  pagination: Pagination;
  setPagination: (params: { page: number; pageSize: number }) => void;
};

export function MemoryTable({
  messages,
  total,
  pagination,
  setPagination,
}: MemoryTableProps) {
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});

  // Define columns for the memory table
  const columns: ColumnDef<IMessageInfo>[] = useMemo(
    () => [
      {
        accessorKey: 'session_id',
        header: () => <span>{t('memoryDetail.messages.sessionId')}</span>,
        cell: ({ row }) => (
          <div className="text-sm font-medium ">
            {row.getValue('session_id')}
          </div>
        ),
      },
      {
        accessorKey: 'agent_name',
        header: () => <span>{t('memoryDetail.messages.agent')}</span>,
        cell: ({ row }) => (
          <div className="text-sm font-medium ">
            {row.getValue('agent_name')}
          </div>
        ),
      },
      {
        accessorKey: 'message_type',
        header: () => <span>{t('memoryDetail.messages.type')}</span>,
        cell: ({ row }) => (
          <div className="text-sm font-medium  capitalize">
            {row.getValue('message_type')}
          </div>
        ),
      },
      {
        accessorKey: 'valid_at',
        header: () => <span>{t('memoryDetail.messages.validDate')}</span>,
        cell: ({ row }) => (
          <div className="text-sm ">{row.getValue('valid_at')}</div>
        ),
      },
      {
        accessorKey: 'forget_at',
        header: () => <span>{t('memoryDetail.messages.forgetAt')}</span>,
        cell: ({ row }) => (
          <div className="text-sm ">{row.getValue('forget_at')}</div>
        ),
      },
      {
        accessorKey: 'source_id',
        header: () => <span>{t('memoryDetail.messages.source')}</span>,
        cell: ({ row }) => (
          <div className="text-sm ">{row.getValue('source_id')}</div>
        ),
      },
      {
        accessorKey: 'status',
        header: () => <span>{t('memoryDetail.messages.enable')}</span>,
        cell: ({ row }) => {
          const isEnabled = row.getValue('status') as boolean;
          return (
            <div className="flex items-center">
              <Switch defaultChecked={isEnabled} onChange={() => {}} />
            </div>
          );
        },
      },
      {
        accessorKey: 'action',
        header: () => <span>{t('memoryDetail.messages.action')}</span>,
        meta: {
          cellClassName: 'w-12',
        },
        cell: () => (
          <div className=" hidden group-hover:flex">
            <Button variant={'ghost'} className="bg-transparent">
              <TextSelect />
            </Button>
            <Button
              variant={'delete'}
              className="bg-transparent"
              aria-label="Edit"
            >
              <Eraser />
            </Button>
          </div>
        ),
      },
    ],
    [],
  );

  const currentPagination = useMemo(() => {
    return {
      pageIndex: (pagination.current || 1) - 1,
      pageSize: pagination.pageSize || 10,
    };
  }, [pagination]);

  const table = useReactTable({
    data: messages,
    columns,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    manualPagination: true,
    state: {
      sorting,
      columnFilters,
      columnVisibility,
      pagination: currentPagination,
    },
    rowCount: total,
  });

  return (
    <div className="w-full">
      <Table rootClassName="max-h-[calc(100vh-222px)]">
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder
                    ? null
                    : flexRender(
                        header.column.columnDef.header,
                        header.getContext(),
                      )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody className="relative">
          {table.getRowModel().rows?.length ? (
            table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                data-state={row.getIsSelected() && 'selected'}
                className="group"
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={columns.length} className="h-24 text-center">
                <Empty type={EmptyType.Data} />
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>

      <div className="flex items-center justify-end py-4 absolute bottom-3 right-3">
        <RAGFlowPagination
          {...pick(pagination, 'current', 'pageSize')}
          total={total}
          onChange={(page, pageSize) => {
            setPagination({ page, pageSize });
          }}
        />
      </div>
    </div>
  );
}
