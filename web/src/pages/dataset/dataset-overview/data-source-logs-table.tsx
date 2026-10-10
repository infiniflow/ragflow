import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { IDataSourceLog } from '@/pages/user-setting/data-source/interface';
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
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LogTabs } from './dataset-common';
import { DatasetOverviewKeys } from './utils';

const DataSourceLogsTable = ({ datasetId }: { datasetId: string }) => {
  const { t: tDatasetOverview } = useTranslation('datasetOverview');
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => {
      setNow(Date.now());
    }, 1000);

    return () => window.clearInterval(timer);
  }, []);

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
    queryFn: async () => {
      const { data: res = {} } = await listDataPipelineLogDocument(datasetId, {
        page: pagination.current,
        page_size: pagination.pageSize,
        log_type: 'datasource',
      });
      return res.data || { logs: [], total: 0 };
    },
  });

  const columns = useMemo(
    () => getDataSourceLogsTableColumns({ now, showDatasetColumn: false }),
    [now],
  );

  const currentPagination = useMemo(
    () => ({
      pageIndex: (pagination.current || 1) - 1,
      pageSize: pagination.pageSize || 10,
    }),
    [pagination],
  );

  const table = useReactTable<IDataSourceLog>({
    data: data?.logs || [],
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
    <div className="flex-1 min-h-0 flex flex-col">
      <Table
        rootClassName="max-h-[calc(100vh-380px)] mb-4"
        className="table-fixed"
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
        <TableBody className="min-w-[1280px]">
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
                <Empty
                  type={EmptyType.Data}
                  text={tDatasetOverview('noData')}
                />
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>

      <div className="mt-auto flex items-center justify-end">
        <RAGFlowPagination
          current={pagination.current}
          pageSize={pagination.pageSize}
          total={data?.total}
          onChange={(page, pageSize) => setPagination({ page, pageSize })}
        />
      </div>
    </div>
  );
};

export default DataSourceLogsTable;
