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

import { FileIcon } from '@/components/icon-font';
import { RenameDialog } from '@/components/rename-dialog';
import { TableEmpty, TableSkeleton } from '@/components/table-skeleton';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
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
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { UseRowSelectionType } from '@/hooks/logic-hooks/use-row-selection';
import { useFetchFileList } from '@/hooks/use-file-request';
import { IFile } from '@/interfaces/database/file-manager';
import { formatFileSize } from '@/utils/common-util';
import { formatDate } from '@/utils/date';
import { pick } from 'lodash';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { ActionCell } from './action-cell';
import { useHandleConnectToKnowledge, useRenameCurrentFile } from './hooks';
import { KnowledgeCell } from './knowledge-cell';
import { LinkToDatasetDialog } from './link-to-dataset-dialog';
import { UseMoveDocumentShowType } from './use-move-file';
import { useNavigateToOtherFolder } from './use-navigate-to-folder';
import { isFolderType, isKnowledgeBaseType } from './util';

declare const __API_PROXY_SCHEME__: string;

type FilesTableProps = Pick<
  ReturnType<typeof useFetchFileList>,
  'files' | 'loading' | 'pagination' | 'setPagination' | 'total'
> &
  Pick<UseRowSelectionType, 'rowSelection' | 'setRowSelection'> &
  UseMoveDocumentShowType;

export function FilesTable({
  files,
  total,
  pagination,
  setPagination,
  loading,
  rowSelection,
  setRowSelection,
  showMoveFileModal,
}: FilesTableProps) {
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});
  const { t } = useTranslation('translation', {
    keyPrefix: 'fileManager',
  });
  const navigateToOtherFolder = useNavigateToOtherFolder();
  const navigate = useNavigate();
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

  // Check if skills feature is enabled (only in hybrid or go mode)
  const isSkillsEnabled = useMemo(() => {
    const scheme =
      typeof __API_PROXY_SCHEME__ !== 'undefined'
        ? __API_PROXY_SCHEME__
        : 'python';
    return scheme === 'hybrid' || scheme === 'go';
  }, []);

  // Sort files with skills folder first, then by time
  // Filter out skills folder if not in hybrid/go mode
  const sortedFiles = useMemo(() => {
    if (!files) return [];

    // Filter out skills folder if feature is disabled
    const filteredFiles = isSkillsEnabled
      ? files
      : files.filter((file) => {
          const isSkills =
            isFolderType(file.type) && file.name.toLowerCase() === 'skills';
          return !isSkills;
        });

    return [...filteredFiles].sort((a, b) => {
      const aIsSkills =
        isFolderType(a.type) && a.name.toLowerCase() === 'skills';
      const bIsSkills =
        isFolderType(b.type) && b.name.toLowerCase() === 'skills';

      // Skills folder always comes first
      if (aIsSkills && !bIsSkills) return -1;
      if (!aIsSkills && bIsSkills) return 1;

      // Then sort by create_time desc (newest first)
      return (b.create_time || 0) - (a.create_time || 0);
    });
  }, [files, isSkillsEnabled]);

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
          disabled={!row.getCanSelect()}
        />
      ),
      enableSorting: false,
      enableHiding: false,
    },
    {
      accessorKey: 'name',
      header: ({ column }) => {
        return (
          <div className="flex items-center gap-1">
            {t('name')}
            <Button
              size="icon-xs"
              variant="ghost"
              onClick={() =>
                column.toggleSorting(column.getIsSorted() === 'asc')
              }
            >
              <ArrowUpDown />
            </Button>
          </div>
        );
      },
      meta: { cellClassName: 'max-w-[20vw]' },
      cell: ({ row }) => {
        const name: string = row.getValue('name');
        const type = row.original.type;
        const id = row.original.id;
        const isFolder = isFolderType(type);
        const isSkillsFolder = isFolder && name.toLowerCase() === 'skills';

        const handleNameClick = () => {
          if (isSkillsFolder) {
            navigate('/files/skills');
          } else if (isFolder) {
            navigateToOtherFolder(id);
          }
        };

        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="static"
                onClick={handleNameClick}
                className="max-w-full p-0 flex justify-start gap-2 text-text-primary"
              >
                <FileIcon name={name} type={isSkillsFolder ? 'skills' : type} />

                <span className="truncate">{name}</span>
              </Button>
            </TooltipTrigger>

            <TooltipContent>{name}</TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: 'create_time',
      header: ({ column }) => {
        return (
          <div className="flex items-center gap-1">
            {t('uploadDate')}
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() =>
                column.toggleSorting(column.getIsSorted() === 'asc')
              }
            >
              <ArrowUpDown />
            </Button>
          </div>
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
          <div className="flex items-center gap-1">
            {t('size')}
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() =>
                column.toggleSorting(column.getIsSorted() === 'asc')
              }
            >
              <ArrowUpDown />
            </Button>
          </div>
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
        const value: IFile['kbs_info'] = row.getValue('kbs_info');
        return <KnowledgeCell value={value}></KnowledgeCell>;
      },
    },
    {
      id: 'actions',
      header: t('action'),
      meta: {
        headerCellClassName: 'w-0 whitespace-nowrap',
      },
      enableHiding: false,
      enablePinning: true,
      cell: ({ row }) => {
        return (
          <ActionCell
            row={row}
            showConnectToKnowledgeModal={showConnectToKnowledgeModal}
            showFileRenameModal={showFileRenameModal}
            showMoveFileModal={showMoveFileModal}
          />
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
    data: sortedFiles,
    columns,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    getCoreRowModel: getCoreRowModel(),
    // getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    getRowId: (row) => row.id, // Use file ID instead of row index
    manualPagination: true, //we're doing manual "server-side" pagination
    enableRowSelection(row) {
      const name = row.original.name;
      const type = row.original.type;
      const isSkillsFolder =
        isFolderType(type) && name.toLowerCase() === 'skills';
      // Skills folder is not selectable when enabled (it's a special entry)
      // When disabled, it's already filtered out
      return !isKnowledgeBaseType(row.original.source_type) && !isSkillsFolder;
    },
    state: {
      sorting,
      columnFilters,
      columnVisibility,
      rowSelection,
      pagination: currentPagination,
    },
    rowCount: total ?? 0,
    debugTable: true,
  });

  return (
    <>
      <div className="flex-1 min-h-0 size-full">
        <Table rootClassName="max-h-full overflow-auto">
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => {
                  return (
                    <TableHead
                      key={header.id}
                      className={
                        header.column.columnDef.meta?.headerCellClassName
                      }
                    >
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
          <TableBody className="max-h-96 overflow-y-auto">
            {loading ? (
              <TableSkeleton columnsLength={columns.length}></TableSkeleton>
            ) : table.getRowModel().rows?.length ? (
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

      <footer className="flex items-center justify-end pb-5 mt-4">
        <RAGFlowPagination
          {...pick(pagination, 'current', 'pageSize')}
          total={total}
          onChange={(page, pageSize) => {
            setPagination({ page, pageSize });
          }}
        />
      </footer>

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
    </>
  );
}
