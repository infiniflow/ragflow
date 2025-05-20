'use client';

import {
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

import { ChunkMethodDialog } from '@/components/chunk-method-dialog';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useFetchNextDocumentList } from '@/hooks/document-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { IDocumentInfo } from '@/interfaces/database/document';
import { getExtension } from '@/utils/document-util';
import { useMemo } from 'react';
import { useChangeDocumentParser } from './hooks';
import { useDatasetTableColumns } from './use-dataset-table-columns';

export function DatasetTable() {
  const {
    // searchString,
    documents,
    pagination,
    // handleInputChange,
    setPagination,
  } = useFetchNextDocumentList();
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});
  const [rowSelection, setRowSelection] = React.useState({});

  const { currentRecord, setRecord } = useSetSelectedRecord<IDocumentInfo>();

  const {
    changeParserLoading,
    onChangeParserOk,
    changeParserVisible,
    hideChangeParserModal,
    showChangeParserModal,
  } = useChangeDocumentParser(currentRecord.id);

  const columns = useDatasetTableColumns({
    showChangeParserModal,
    setCurrentRecord: setRecord,
  });

  const currentPagination = useMemo(() => {
    return {
      pageIndex: (pagination.current || 1) - 1,
      pageSize: pagination.pageSize || 10,
    };
  }, [pagination]);

  const table = useReactTable({
    data: documents,
    columns,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
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
    rowCount: pagination.total ?? 0,
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
            {table.getRowModel().rows?.length ? (
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
          {table.getFilteredSelectedRowModel().rows.length} of{' '}
          {pagination?.total} row(s) selected.
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
      {changeParserVisible && (
        <ChunkMethodDialog
          documentId={currentRecord.id}
          parserId={currentRecord.parser_id}
          parserConfig={currentRecord.parser_config}
          documentExtension={getExtension(currentRecord.name)}
          onOk={onChangeParserOk}
          visible={changeParserVisible}
          hideModal={hideChangeParserModal}
          loading={changeParserLoading}
        ></ChunkMethodDialog>
      )}
    </div>
  );
}
