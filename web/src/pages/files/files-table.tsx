'use client';

import {
  ColumnDef,
  ColumnFiltersState,
  SortingState,
  VisibilityState,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { ArrowUpDown } from 'lucide-react';
import * as React from 'react';

import { RenameDialog } from '@/components/rename-dialog';
import SvgIcon from '@/components/svg-icon';
import { TableEmpty, TableSkeleton } from '@/components/table-skeleton';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useFetchFileList } from '@/hooks/file-manager-hooks';
import { IFile } from '@/interfaces/database/file-manager';
import { cn } from '@/lib/utils';
import { formatFileSize } from '@/utils/common-util';
import { formatDate } from '@/utils/date';
import { getExtension } from '@/utils/document-util';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ActionCell } from './action-cell';
import {
  useHandleConnectToKnowledge,
  useNavigateToOtherFolder,
  useRenameCurrentFile,
} from './hooks';
import { LinkToDatasetDialog } from './link-to-dataset-dialog';

export function FilesTable() {
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});
  const [rowSelection, setRowSelection] = React.useState({});
  const { t } = useTranslation('translation', {
    keyPrefix: 'fileManager',
  });
  const navigateToOtherFolder = useNavigateToOtherFolder();
  const {
    connectToKnowledgeVisible,
    hideConnectToKnowledgeModal,
    showConnectToKnowledgeModal,
    initialConnectedIds,
    onConnectToKnowledgeOk,
    connectToKnowledgeLoading,
  } = useHandleConnectToKnowledge();
  const {
    fileRenameVisible,
    showFileRenameModal,
    hideFileRenameModal,
    onFileRenameOk,
    initialFileName,
    fileRenameLoading,
  } = useRenameCurrentFile();

  const { pagination, data, loading, setPagination } = useFetchFileList();

  const columns: ColumnDef<IFile>[] = [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={
            table.getIsAllPageRowsSelected() ||
            (table.getIsSomePageRowsSelected() && 'indeterminate')
          }
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label="Select all"
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label="Select row"
        />
      ),
      enableSorting: false,
      enableHiding: false,
    },
    {
      accessorKey: 'name',
      header: ({ column }) => {
        return (
          <Button
            variant="ghost"
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          >
            {t('name')}
            <ArrowUpDown />
          </Button>
        );
      },
      meta: { cellClassName: 'max-w-[20vw]' },
      cell: ({ row }) => {
        const name: string = row.getValue('name');
        const type = row.original.type;
        const id = row.original.id;
        const isFolder = type === 'folder';

        const handleNameClick = () => {
          if (isFolder) {
            navigateToOtherFolder(id);
          }
        };

        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="flex gap-2">
                <SvgIcon
                  name={`file-icon/${isFolder ? 'folder' : getExtension(name)}`}
                  width={24}
                ></SvgIcon>
                <span
                  className={cn('truncate', { ['cursor-pointer']: isFolder })}
                  onClick={handleNameClick}
                >
                  {name}
                </span>
              </div>
            </TooltipTrigger>
            <TooltipContent>
              <p>{name}</p>
            </TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: 'create_time',
      header: ({ column }) => {
        return (
          <Button
            variant="ghost"
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          >
            {t('uploadDate')}
            <ArrowUpDown />
          </Button>
        );
      },
      cell: ({ row }) => (
        <div className="lowercase">
          {formatDate(row.getValue('create_time'))}
        </div>
      ),
    },
    {
      accessorKey: 'size',
      header: ({ column }) => {
        return (
          <Button
            variant="ghost"
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          >
            {t('size')}
            <ArrowUpDown />
          </Button>
        );
      },
      cell: ({ row }) => (
        <div className="capitalize">{formatFileSize(row.getValue('size'))}</div>
      ),
    },
    {
      accessorKey: 'kbs_info',
      header: t('knowledgeBase'),
      cell: ({ row }) => {
        const value = row.getValue('kbs_info');
        return Array.isArray(value) ? (
          <section className="flex gap-2 items-center">
            {value?.slice(0, 2).map((x) => (
              <Badge key={x.kb_id} className="" variant={'tertiary'}>
                {x.kb_name}
              </Badge>
            ))}

            {value.length > 2 && (
              <Button variant={'icon'} size={'auto'}>
                +{value.length - 2}
              </Button>
            )}
          </section>
        ) : (
          ''
        );
      },
    },
    {
      id: 'actions',
      header: t('action'),
      enableHiding: false,
      cell: ({ row }) => {
        return (
          <ActionCell
            row={row}
            showConnectToKnowledgeModal={showConnectToKnowledgeModal}
            showFileRenameModal={showFileRenameModal}
          ></ActionCell>
        );
      },
    },
  ];

  const currentPagination = useMemo(() => {
    return {
      pageIndex: (pagination.current || 1) - 1,
      pageSize: pagination.pageSize || 10,
    };
  }, [pagination]);

  const table = useReactTable({
    data: data?.files || [],
    columns,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    getCoreRowModel: getCoreRowModel(),
    // getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    onPaginationChange: (updaterOrValue: any) => {
      if (typeof updaterOrValue === 'function') {
        const nextPagination = updaterOrValue(currentPagination);
        setPagination({
          page: nextPagination.pageIndex + 1,
          pageSize: nextPagination.pageSize,
        });
      } else {
        setPagination({
          page: updaterOrValue.pageIndex,
          pageSize: updaterOrValue.pageSize,
        });
      }
    },
    manualPagination: true, //we're doing manual "server-side" pagination

    state: {
      sorting,
      columnFilters,
      columnVisibility,
      rowSelection,
      pagination: currentPagination,
    },
    rowCount: data?.total ?? 0,
    debugTable: true,
  });

  return (
    <div className="w-full">
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => {
                  return (
                    <TableHead key={header.id}>
                      {header.isPlaceholder
                        ? null
                        : flexRender(
                            header.column.columnDef.header,
                            header.getContext(),
                          )}
                    </TableHead>
                  );
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableSkeleton columnsLength={columns.length}></TableSkeleton>
            ) : table.getRowModel().rows?.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  key={row.id}
                  data-state={row.getIsSelected() && 'selected'}
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell
                      key={cell.id}
                      className={cell.column.columnDef.meta?.cellClassName}
                    >
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableEmpty columnsLength={columns.length}></TableEmpty>
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex items-center justify-end space-x-2 py-4">
        <div className="flex-1 text-sm text-muted-foreground">
          {table.getFilteredSelectedRowModel().rows.length} of {data?.total}{' '}
          row(s) selected.
        </div>
        <div className="space-x-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
          >
            Next
          </Button>
        </div>
      </div>
      {connectToKnowledgeVisible && (
        <LinkToDatasetDialog
          hideModal={hideConnectToKnowledgeModal}
          initialConnectedIds={initialConnectedIds}
          onConnectToKnowledgeOk={onConnectToKnowledgeOk}
          loading={connectToKnowledgeLoading}
        ></LinkToDatasetDialog>
      )}
      {fileRenameVisible && (
        <RenameDialog
          hideModal={hideFileRenameModal}
          onOk={onFileRenameOk}
          initialName={initialFileName}
          loading={fileRenameLoading}
        ></RenameDialog>
      )}
    </div>
  );
}
