import FileStatusBadge from '@/components/file-status-badge';
import { FileIcon } from '@/components/icon-font';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import SvgIcon from '@/components/svg-icon';
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
import { useTranslate } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import ProcessLogModal from '@/pages/datasets/process-log-modal';
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
import { TFunction } from 'i18next';
import { ClipboardList, Eye } from 'lucide-react';
import { Dispatch, FC, SetStateAction, useMemo, useState } from 'react';
import { LogTabs, processingType } from './dataset-common';

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
  active: (typeof LogTabs)[keyof typeof LogTabs];
}

export const getFileLogsTableColumns = (
  t: TFunction<'translation', string>,
  setIsModalVisible: Dispatch<SetStateAction<boolean>>,
  navigateToDataflowResult: (
    id: string,
    knowledgeId?: string | undefined,
  ) => () => void,
) => {
  // const { t } = useTranslate('knowledgeDetails');
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
      header: t('fileName'),
      cell: ({ row }) => (
        <div
          className="flex items-center gap-2 text-text-primary"
          onClick={navigateToDataflowResult(
            row.original.id,
            row.original.kb_id,
          )}
        >
          <FileIcon name={row.original.fileName}></FileIcon>
          {row.original.fileName}
        </div>
      ),
    },
    {
      accessorKey: 'source',
      header: t('source'),
      cell: ({ row }) => (
        <div className="text-text-primary">{row.original.source}</div>
      ),
    },
    {
      accessorKey: 'pipeline',
      header: t('dataPipeline'),
      cell: ({ row }) => (
        <div className="flex items-center gap-2 text-text-primary">
          <RAGFlowAvatar
            avatar={null}
            name={row.original.pipeline}
            className="size-4"
          />
          {row.original.pipeline}
        </div>
      ),
    },
    {
      accessorKey: 'startDate',
      header: t('startDate'),
      cell: ({ row }) => (
        <div className="text-text-primary">{row.original.startDate}</div>
      ),
    },
    {
      accessorKey: 'task',
      header: t('task'),
      cell: ({ row }) => (
        <div className="text-text-primary">{row.original.task}</div>
      ),
    },
    {
      accessorKey: 'status',
      header: t('status'),
      cell: ({ row }) => <FileStatusBadge status={row.original.status} />,
    },
    {
      id: 'operations',
      header: t('operations'),
      cell: ({ row }) => (
        <div className="flex justify-start space-x-2">
          <Button
            variant="ghost"
            size="sm"
            className="p-1"
            onClick={() => {
              setIsModalVisible(true);
            }}
          >
            <Eye />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="p-1"
            onClick={navigateToDataflowResult(row.original.id)}
          >
            <ClipboardList />
          </Button>
        </div>
      ),
    },
  ];

  return columns;
};

export const getDatasetLogsTableColumns = (
  t: TFunction<'translation', string>,
  setIsModalVisible: Dispatch<SetStateAction<boolean>>,
) => {
  // const { t } = useTranslate('knowledgeDetails');
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
      accessorKey: 'startDate',
      header: t('startDate'),
      cell: ({ row }) => (
        <div className="text-text-primary">{row.original.startDate}</div>
      ),
    },
    {
      accessorKey: 'processingType',
      header: t('processingType'),
      cell: ({ row }) => (
        <div className="flex items-center gap-2 text-text-primary">
          {processingType.knowledgeGraph === row.original.processingType && (
            <SvgIcon name={`data-flow/knowledgegraph`} width={24}></SvgIcon>
          )}
          {processingType.raptor === row.original.processingType && (
            <SvgIcon name={`data-flow/raptor`} width={24}></SvgIcon>
          )}
          {row.original.processingType}
        </div>
      ),
    },
    {
      accessorKey: 'status',
      header: t('status'),
      cell: ({ row }) => <FileStatusBadge status={row.original.status} />,
    },
    {
      id: 'operations',
      header: t('operations'),
      cell: ({ row }) => (
        <div className="flex justify-start space-x-2">
          <Button
            variant="ghost"
            size="sm"
            className="p-1"
            onClick={() => {
              setIsModalVisible(true);
            }}
          >
            <Eye />
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
  active = LogTabs.FILE_LOGS,
}) => {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [rowSelection, setRowSelection] = useState({});
  const { t } = useTranslate('knowledgeDetails');
  const [isModalVisible, setIsModalVisible] = useState(false);
  const { navigateToDataflowResult } = useNavigatePage();
  const columns = useMemo(() => {
    console.log('columns', active);
    return active === LogTabs.FILE_LOGS
      ? getFileLogsTableColumns(t, setIsModalVisible, navigateToDataflowResult)
      : getDatasetLogsTableColumns(t, setIsModalVisible);
  }, [active, t]);

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
  const taskInfo = {
    taskId: '#9527',
    fileName: 'PRD for DealBees 1.2 (1).text',
    fileSize: '2.4G',
    source: 'Github',
    task: 'Parse',
    state: 'Running',
    startTime: '14/03/2025 14:53:39',
    duration: '800',
    details: 'PRD for DealBees 1.2 (1).text',
  };
  return (
    <div className="w-full h-[calc(100vh-350px)]">
      <Table rootClassName="max-h-[calc(100vh-380px)]">
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
      <div className="flex items-center justify-end py-4 absolute bottom-3 right-12">
        <div className="space-x-2">
          <RAGFlowPagination
            {...{ current: pagination.current, pageSize: pagination.pageSize }}
            total={pagination.total}
            onChange={(page, pageSize) => setPagination({ page, pageSize })}
          />
        </div>
      </div>
      <ProcessLogModal
        visible={isModalVisible}
        onCancel={() => setIsModalVisible(false)}
        taskInfo={taskInfo}
      />
    </div>
  );
};

export default FileLogsTable;
