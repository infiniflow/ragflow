import FileStatusBadge from '@/components/file-status-badge';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { RunningStatusMap } from '@/constants/knowledge';
import { RunningStatus } from '@/pages/dataset/dataset/constant';
import { Routes } from '@/routes';
import { formatDate } from '@/utils/date';
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { t } from 'i18next';
import { pick } from 'lodash';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useLogListDataSource } from '../hooks';
import { IDataSourceLog } from '../interface';

const formatDuration = (seconds: number) => {
  const safeSeconds = Math.max(0, seconds);
  const hours = Math.floor(safeSeconds / 3600);
  const minutes = Math.floor((safeSeconds % 3600) / 60);
  const remainingSeconds = safeSeconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m ${remainingSeconds}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${remainingSeconds}s`;
  }
  return `${remainingSeconds}s`;
};

const getTaskCountdownSeconds = (row: IDataSourceLog, now: number) => {
  const freqMinutes =
    row.task_type === 'prune'
      ? Number(row.prune_freq || 0)
      : Number(row.refresh_freq || 0);
  const scheduledAt = row.time_started
    ? new Date(row.time_started).getTime()
    : 0;

  if (!freqMinutes || !scheduledAt) {
    return null;
  }

  const nextRunAt = scheduledAt + freqMinutes * 60 * 1000;
  return Math.ceil((nextRunAt - now) / 1000);
};

const TaskCountdown = ({ row, now }: { row: IDataSourceLog; now: number }) => {
  const remainingSeconds = getTaskCountdownSeconds(row, now);

  if (remainingSeconds === null) {
    return '';
  }

  return <span>Task starts in {formatDuration(remainingSeconds)}</span>;
};

const getSummary = (row: IDataSourceLog, now: number) => {
  if (row.status === RunningStatus.SCHEDULE || row.status === '5') {
    return <TaskCountdown row={row} now={now} />;
  }

  if (row.status === RunningStatus.RUNNING || row.status === '1') {
    return row.task_type === 'prune' ? 'Prune in progress' : 'Sync in progress';
  }

  if (row.status === RunningStatus.FAIL || row.status === '4') {
    return row.error_msg || 'Task failed';
  }

  if (row.status === RunningStatus.CANCEL || row.status === '2') {
    return '';
  }

  if (row.task_type === 'prune') {
    return `deleted=${row.docs_removed_from_index || 0}, error=${row.error_count || 0}`;
  }

  return `total=${row.total_docs_indexed || 0}, added=${row.new_docs_indexed || 0}, updated=${Math.max(
    0,
    (row.total_docs_indexed || 0) - (row.new_docs_indexed || 0),
  )}, error=${row.error_count || 0}`;
};

const columns = ({
  handleToDataSetDetail,
  now,
}: {
  handleToDataSetDetail: (id: string) => void;
  now: number;
}) => {
  return [
    {
      accessorKey: 'update_date',
      header: t('setting.timeStarted'),
      cell: ({ row }) => (
        <div className="flex items-center gap-2 text-text-primary">
          {row.original.update_date
            ? formatDate(row.original.update_date)
            : '-'}
        </div>
      ),
    },
    {
      accessorKey: 'status',
      header: t('knowledgeDetails.status'),
      cell: ({ row }) => (
        <FileStatusBadge
          status={row.original.status as RunningStatus}
          name={RunningStatusMap[row.original.status as RunningStatus]}
          className="!w-20"
        />
      ),
    },
    {
      accessorKey: 'kb_name',
      header: t('knowledgeDetails.dataset'),
      cell: ({ row }) => {
        return (
          <div
            className="flex items-center gap-2 text-text-primary cursor-pointer"
            onClick={() => {
              handleToDataSetDetail(row.original.kb_id);
            }}
          >
            <RAGFlowAvatar
              avatar={row.original.avatar}
              name={row.original.kb_name}
              className="size-4"
            />
            {row.original.kb_name}
          </div>
        );
      },
    },
    {
      accessorKey: 'task_type',
      header: 'Task Type',
      cell: ({ row }) => row.original.task_type || 'sync',
    },
    {
      id: 'summary',
      header: 'Summary',
      cell: ({ row }) => (
        <div className="max-w-[32rem] whitespace-normal break-words text-text-primary">
          {getSummary(row.original as IDataSourceLog, now)}
        </div>
      ),
    },
  ] as ColumnDef<any>[];
};

// const paginationInit = {
//   current: 1,
//   pageSize: 10,
//   total: 0,
// };
export const DataSourceLogsTable = ({
  autoRefresh,
}: {
  autoRefresh: boolean;
}) => {
  const { data, pagination, setPagination } = useLogListDataSource(autoRefresh);
  const navigate = useNavigate();
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => {
      setNow(Date.now());
    }, 1000);

    return () => window.clearInterval(timer);
  }, []);

  const currentPagination = useMemo(
    () => ({
      pageIndex: (pagination.current || 1) - 1,
      pageSize: pagination.pageSize || 10,
    }),
    [pagination],
  );

  const handleToDataSetDetail = useCallback(
    (id: string) => {
      navigate(`${Routes.Dataset}/${id}`);
    },
    [navigate],
  );

  const table = useReactTable<any>({
    data: data || [],
    columns: columns({ handleToDataSetDetail, now }),
    manualPagination: true,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    // onSortingChange: setSorting,
    // onColumnFiltersChange: setColumnFilters,
    // onRowSelectionChange: setRowSelection,
    state: {
      //   sorting,
      //   columnFilters,
      //   rowSelection,
      pagination: currentPagination,
    },
    // pageCount: pagination.total
    //   ? Math.ceil(pagination.total / pagination.pageSize)
    //   : 0,
    rowCount: pagination.total ?? 0,
  });

  return (
    // <div className="w-full h-[calc(100vh-360px)]">
    //   <Table rootClassName="max-h-[calc(100vh-380px)]">
    <div className="w-full">
      <Table>
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
        <TableBody className="relative min-w-[1280px] overflow-auto">
          {table.getRowModel().rows?.length ? (
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
              <TableCell colSpan={5} className="h-24 text-center">
                {t('common.noData')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      <div className="flex items-center justify-end mt-4">
        <div className="space-x-2">
          {/* <RAGFlowPagination
            {...{ current: pagination.current, pageSize: pagination.pageSize }}
            total={pagination.total}
            onChange={(page, pageSize) => setPagination({ page, pageSize })}
          /> */}
          <RAGFlowPagination
            {...pick(pagination, 'current', 'pageSize')}
            total={pagination.total}
            onChange={(page, pageSize) => {
              setPagination({ page, pageSize });
            }}
          ></RAGFlowPagination>
        </div>
      </div>
    </div>
  );
};
