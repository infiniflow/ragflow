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
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { UseRowSelectionType } from '@/hooks/logic-hooks/use-row-selection';
import {
  useFetchDocumentList,
  useGetDocumentFilter,
} from '@/hooks/use-document-request';
import dayjs from 'dayjs';
import { pick } from 'lodash';
import { useMemo } from 'react';
import { PaginatedTableLayout } from '@/components/paginated-table-layout';
import { ResourceMetadata } from '@/components/resource-metadata';
import { ShowManageMetadataModalProps } from '../components/metedata/interface';
import { hasManagePermissionPermission } from '@/ee/utils/permission-util';
import ProcessLogModal from '../process-log-modal';
import {
  ChangeParserDialog,
  ReparseConfirmDialog,
} from './change-parser-dialog';
import { useShowLog } from './hooks';
import { useChangeDocumentParser } from './use-change-document-parser';
import { useDatasetTableColumns } from './use-dataset-table-columns';
import { hasEditPermission } from '@/ee/utils/permission-util';
import { useRenameDocument } from './use-rename-document';
import { useKnowledgeBaseContext } from '../contexts/knowledge-base-context';
import { formatBytes } from '@/utils/file-util';
import { useTranslation } from 'react-i18next';

function formatDatasetCreateTime(createTime?: number) {
  if (!createTime) {
    return '';
  }
  return dayjs(createTime).format('DD/MM/YYYY HH:mm:ss');
}

export type DatasetTableProps = Pick<
  ReturnType<typeof useFetchDocumentList>,
  'documents' | 'setPagination' | 'pagination' | 'loading'
> &
  Pick<UseRowSelectionType, 'rowSelection' | 'setRowSelection'> & {
    showManageMetadataModal: (config: ShowManageMetadataModalProps) => void;
  };

export function DatasetTable({
  documents,
  pagination,
  setPagination,
  rowSelection,
  setRowSelection,
  showManageMetadataModal,
}: DatasetTableProps) {
  const { t } = useTranslation();
  const { knowledgeBase } = useKnowledgeBaseContext();
  const { parsingCount, pendingCount } = useGetDocumentFilter();
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
    reparseVisible,
    hideReparseModal,
    confirmReparse,
  } = useChangeDocumentParser();

  const {
    renameLoading,
    onRenameOk,
    renameVisible,
    hideRenameModal,
    showRenameModal,
    initialName,
  } = useRenameDocument();

  const { showLog, logInfo, logVisible, hideLog, datasetId, logId } =
    useShowLog(documents);

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
    enableRowSelection: (row) =>
      hasEditPermission(row.original.operator_permission ?? 0),
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

  const metadataItems = [
    {
      key: 'size',
      content: formatBytes(knowledgeBase?.size ?? 0),
    },
    {
      key: 'pending',
      content: t('knowledgeDetails.filesPending', { count: pendingCount }),
    },
    {
      key: 'parsing',
      content: t('knowledgeDetails.filesParsing', { count: parsingCount }),
    },
  ];

  return (
    <>
      <PaginatedTableLayout
        footerLeading={
          <ResourceMetadata
            createTime={formatDatasetCreateTime(knowledgeBase?.create_time)}
            items={metadataItems}
          />
        }
        pagination={
          <RAGFlowPagination
            {...pick(pagination, 'current', 'pageSize')}
            total={pagination.total}
            onChange={(page, pageSize) => {
              setPagination({ page, pageSize });
            }}
          ></RAGFlowPagination>
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
      </PaginatedTableLayout>
      {changeParserVisible && (
        <ChangeParserDialog
          record={changeParserRecord}
          onOk={onChangeParserOk}
          hideModal={hideChangeParserModal}
          loading={changeParserLoading}
          readOnly={
            !hasManagePermissionPermission(
              changeParserRecord.operator_permission ?? 0,
            )
          }
        />
      )}

      {reparseVisible && (
        <ReparseConfirmDialog
          visible={reparseVisible}
          onOk={confirmReparse}
          onCancel={hideReparseModal}
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
          datasetId={datasetId}
          logId={logId}
        />
      )}
    </>
  );
}
