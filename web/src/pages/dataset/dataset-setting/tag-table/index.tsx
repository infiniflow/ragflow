'use client';

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
import { ArrowUpDown, Pencil, Trash2 } from 'lucide-react';
import * as React from 'react';

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
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
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useDeleteTag, useFetchTagList } from '@/hooks/use-knowledge-request';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRenameKnowledgeTag } from '../hooks';
import { RenameDialog } from './rename-dialog';

export type ITag = {
  tag: string;
  frequency: number;
};

export function TagTable() {
  const { t } = useTranslation();
  const { list } = useFetchTagList();
  const [tagList, setTagList] = useState<ITag[]>([]);

  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});
  const [rowSelection, setRowSelection] = useState({});

  const { deleteTag } = useDeleteTag();

  useEffect(() => {
    setTagList(list.map((x) => ({ tag: x[0], frequency: x[1] })));
  }, [list]);

  const handleDeleteTag = useCallback(
    (tags: string[]) => () => {
      deleteTag(tags);
    },
    [deleteTag],
  );

  const {
    showTagRenameModal,
    hideTagRenameModal,
    tagRenameVisible,
    initialName,
  } = useRenameKnowledgeTag();

  const columns: ColumnDef<ITag>[] = [
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
      accessorKey: 'tag',
      header: ({ column }) => {
        return (
          <Button
            variant="ghost"
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          >
            {t('knowledgeConfiguration.tagName')}
            <ArrowUpDown />
          </Button>
        );
      },
      cell: ({ row }) => {
        const value: string = row.getValue('tag');
        return <div>{value}</div>;
      },
    },
    {
      accessorKey: 'frequency',
      header: ({ column }) => {
        return (
          <Button
            variant="ghost"
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          >
            {t('knowledgeConfiguration.frequency')}
            <ArrowUpDown />
          </Button>
        );
      },
      cell: ({ row }) => (
        <div className="capitalize ">{row.getValue('frequency')}</div>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: t('common.action'),
      cell: ({ row }) => {
        return (
          <div className="flex gap-1">
            <Tooltip>
              <ConfirmDeleteDialog onOk={handleDeleteTag([row.original.tag])}>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon">
                    <Trash2 />
                  </Button>
                </TooltipTrigger>
              </ConfirmDeleteDialog>
              <TooltipContent>
                <p>{t('common.delete')}</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => showTagRenameModal(row.original.tag)}
                >
                  <Pencil />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>{t('common.rename')}</p>
              </TooltipContent>
            </Tooltip>
          </div>
        );
      },
    },
  ];

  const table = useReactTable({
    data: tagList,
    columns,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    state: {
      sorting,
      columnFilters,
      columnVisibility,
      rowSelection,
    },
  });

  const selectedRowLength = table.getFilteredSelectedRowModel().rows.length;

  return (
    <TooltipProvider>
      <div className="w-full">
        <div className="flex items-center justify-between py-4 ">
          <Input
            placeholder={t('knowledgeConfiguration.searchTags')}
            value={(table.getColumn('tag')?.getFilterValue() as string) ?? ''}
            onChange={(event) =>
              table.getColumn('tag')?.setFilterValue(event.target.value)
            }
            className="w-1/2"
          />
          {selectedRowLength > 0 && (
            <ConfirmDeleteDialog
              onOk={handleDeleteTag(
                table
                  .getFilteredSelectedRowModel()
                  .rows.map((x) => x.original.tag),
              )}
            >
              <Button variant="outline" size="icon">
                <Trash2 />
              </Button>
            </ConfirmDeleteDialog>
          )}
        </div>
        <Table rootClassName="rounded-none border max-h-80 overflow-y-auto">
          <TableHeader className="bg-[#39393b]">
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
            {table.getRowModel().rows?.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  key={row.id}
                  data-state={row.getIsSelected() && 'selected'}
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className="h-24 text-center"
                >
                  No results.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex items-center justify-end space-x-2 py-4">
        <div className="flex-1 text-sm text-muted-foreground">
          {selectedRowLength} of {table.getFilteredRowModel().rows.length}{' '}
          row(s) selected.
        </div>
        <div className="space-x-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
          >
            {t('common.previousPage')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
          >
            {t('common.nextPage')}
          </Button>
        </div>
      </div>
      {tagRenameVisible && (
        <RenameDialog
          hideModal={hideTagRenameModal}
          initialName={initialName}
        ></RenameDialog>
      )}
    </TooltipProvider>
  );
}
