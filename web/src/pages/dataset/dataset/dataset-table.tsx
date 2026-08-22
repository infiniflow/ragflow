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

import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { RenameDialog } from '@/components/rename-dialog';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { UseRowSelectionType } from '@/hooks/logic-hooks/use-row-selection';
import { useFetchDocumentList } from '@/hooks/use-document-request';
import { t } from 'i18next';
import { pick } from 'lodash';
import { useMemo } from 'react';
import { ShowManageMetadataModalProps } from '../components/metedata/interface';
import ProcessLogModal from '../process-log-modal';
import { ChangeParserDialog } from './change-parser-dialog';
import { useShowLog } from './hooks';
import { useChangeDocumentParser } from './use-change-document-parser';
import { useDatasetTableColumns } from './use-dataset-table-columns';
import { useRenameDocument } from './use-rename-document';

export type DatasetTableProps = Pick<
  ReturnType<typeof useFetchDocumentList>,
  'documents' | 'setPagination' | 'pagination' | 'loading'
> &
  Pick<UseRowSelectionType, 'rowSelection' | 'setRowSelection'> & {
    showManageMetadataModal: (config: ShowManageMetadataModalProps) => void;
    bulkOperateBarVisible?: boolean;
  };

export function DatasetTable({
  documents,
  pagination,
  setPagination,
  rowSelection,
  setRowSelection,
  showManageMetadataModal,
  bulkOperateBarVisible = false,
}: DatasetTableProps) {
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});

  const {
    changeParserLoading,
    onChangeParserOk,
    changeParserVisible,
    hideChangeParserModal,
    showChangeParserModal,
    changeParserRecord,
  } = useChangeDocumentParser();

  const {
    renameLoading,
    onRenameOk,
    renameVisible,
    hideRenameModal,
    showRenameModal,
    initialName,
  } = useRenameDocument();

  const { showLog, logInfo, logVisible, hideLog } = useShowLog(documents);

  const columns = useDatasetTableColumns({
    showChangeParserModal,
    showRenameModal,
    showManageMetadataModal,
    showLog,
    setRowSelection,
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
    getRowId: (row) => row.id, // Use document ID instead of row index
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
      <Table
        rootClassName={
          bulkOperateBarVisible
            ? 'max-h-[calc(100vh-320px)]'
            : 'max-h-[calc(100vh-280px)]'
        }
      >
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
        <TableBody className="relative">
          {table.getRowModel().rows?.length ? (
            table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                data-testid="document-row"
                data-doc-name={row.original.name}
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
                <Empty type={EmptyType.Data} />
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      <div className="flex items-center justify-end  py-4 absolute bottom-3 right-8">
        <div className="space-x-2">
          <RAGFlowPagination
            {...pick(pagination, 'current', 'pageSize')}
            total={pagination.total}
            onChange={(page, pageSize) => {
              setPagination({ page, pageSize });
            }}
          ></RAGFlowPagination>
        </div>
      </div>
      {changeParserVisible && (
        <ChangeParserDialog
          record={changeParserRecord}
          visible={changeParserVisible}
          onOk={onChangeParserOk}
          hideModal={hideChangeParserModal}
          loading={changeParserLoading}
        />
      )}

      {renameVisible && (
        <RenameDialog
          visible={renameVisible}
          onOk={onRenameOk}
          loading={renameLoading}
          hideModal={hideRenameModal}
          initialName={initialName}
        ></RenameDialog>
      )}

      {logVisible && (
        <ProcessLogModal
          title={t('knowledgeDetails.fileLogs')}
          visible={logVisible}
          onCancel={() => hideLog()}
          logInfo={logInfo}
        />
      )}
    </div>
  );
}
