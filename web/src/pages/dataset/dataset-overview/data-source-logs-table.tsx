import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { RunningStatus, RunningStatusOld } from '@/constants/knowledge';
import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { IDataSourceLog } from '@/pages/user-setting/data-source/interface';
import { useListDataSource } from '@/pages/user-setting/data-source/hooks';
import { getDataSourceLogsTableColumns } from '@/pages/user-setting/data-source/log-columns';
import { listDataPipelineLogDocument } from '@/services/knowledge-service';
import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { PaginatedTableLayout } from '@/components/paginated-table-layout';
import { LogTabs } from './dataset-common';
import { DatasetOverviewKeys } from './utils';

const PollIntervalMs = 5000;

// Backend variants differ: Python emits "SCHEDULE", Go emits "5".
const isScheduledRow = (log: IDataSourceLog) => {
  const status = log.status as string;
  return (
    status === RunningStatus.SCHEDULE || status === RunningStatusOld.SCHEDULE
  );
};

const DataSourceLogsTable = ({ datasetId }: { datasetId: string }) => {
  const { t: tDatasetOverview } = useTranslation('datasetOverview');
  const { pagination, setPagination } = useGetPaginationWithRouter();

  // Resolve the syncing data source's name from its connector_id: the sync log
  // rows only carry connector_id, so join against the connector list.
  const { list: connectors } = useListDataSource();
  const sourceNameMap = useMemo(() => {
    const map: Record<string, string> = {};
    (connectors || []).forEach((connector) => {
      map[connector.id] = connector.name;
    });
    return map;
  }, [connectors]);

  const { data } = useQuery<{ logs: IDataSourceLog[]; total: number }>({
    queryKey: DatasetOverviewKeys.logs(
      datasetId,
      pagination.current,
      pagination.pageSize,
      '',
      LogTabs.DATASOURCE_LOGS,
      {},
    ),
    placeholderData: (previousData) => previousData ?? { logs: [], total: 0 },
    enabled: !!datasetId,
    // Refetch immediately whenever the tab is shown so the user always
    // sees the latest sync state on entry.
    refetchOnMount: 'always',
    // Poll at a fixed cadence while the tab is open.
    refetchInterval: PollIntervalMs,
    queryFn: async () => {
      const { data: res = {} } = await listDataPipelineLogDocument(datasetId, {
        page: pagination.current,
        page_size: pagination.pageSize,
        log_type: 'datasource',
      });
      return res.data || { logs: [], total: 0 };
    },
  });

  // Scheduled rows (the ones rendering the "Task starts in" countdown)
  // are hidden here: the dataset view is a completed-history log, the
  // live schedule belongs on the personal data source detail page.
  const visibleLogs = useMemo(
    () => (data?.logs || []).filter((log) => !isScheduledRow(log)),
    [data?.logs],
  );

  // Static `now`: with scheduled rows filtered out no cell renders a
  // live countdown, so the table doesn't need a 1s re-render tick.
  const columns = useMemo(
    () =>
      getDataSourceLogsTableColumns({
        now: Date.now(),
        showDatasetColumn: false,
        getSourceName: (connectorId) => sourceNameMap[connectorId],
      }),
    [sourceNameMap],
  );

  const currentPagination = useMemo(
    () => ({
      pageIndex: (pagination.current || 1) - 1,
      pageSize: pagination.pageSize || 10,
    }),
    [pagination],
  );

  const table = useReactTable<IDataSourceLog>({
    data: visibleLogs,
    columns,
    manualPagination: true,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    state: {
      pagination: currentPagination,
    },
    rowCount: data?.total ?? 0,
  });

  return (
    <PaginatedTableLayout
      tableClassName="table-fixed"
      pagination={
        <RAGFlowPagination
          current={pagination.current}
          pageSize={pagination.pageSize}
          total={data?.total}
          onChange={(page, pageSize) => setPagination({ page, pageSize })}
        />
      }
    >
      <TableHeader>
        {table.getHeaderGroups().map((headerGroup) => (
          <TableRow key={headerGroup.id}>
            {headerGroup.headers.map((header) => (
              <TableHead
                key={header.id}
                className={header.column.columnDef.meta?.headerClassName}
              >
                {flexRender(
                  header.column.columnDef.header,
                  header.getContext(),
                )}
              </TableHead>
            ))}
          </TableRow>
        ))}
      </TableHeader>
      <TableBody>
        {table.getRowModel().rows?.length ? (
          table.getRowModel().rows.map((row) => (
            <TableRow key={row.id}>
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
              <Empty type={EmptyType.Data} text={tDatasetOverview('noData')} />
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </PaginatedTableLayout>
  );
};

export default DataSourceLogsTable;
