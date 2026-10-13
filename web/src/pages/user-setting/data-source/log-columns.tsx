import FileStatusBadge from '@/components/file-status-badge';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { RunningStatusMap } from '@/constants/knowledge';
import { RunningStatus } from '@/pages/dataset/dataset/constant';
import { formatDate } from '@/utils/date';
import { ColumnDef } from '@tanstack/react-table';
import { t } from 'i18next';
import { IDataSourceLog } from './interface';
import { getSummary } from './log-summary';

// The data source sync log table is shared by the data source detail page and
// the dataset logs page. The dataset page already scopes every row to one
// knowledge base, so it renders the table without the dataset column.
export const getDataSourceLogsTableColumns = ({
  now,
  showDatasetColumn = true,
  handleToDataSetDetail,
}: {
  now: number;
  showDatasetColumn?: boolean;
  handleToDataSetDetail?: (id: string) => void;
}): ColumnDef<IDataSourceLog>[] => {
  const columns: ColumnDef<IDataSourceLog>[] = [
    {
      accessorKey: 'update_date',
      header: t('setting.timeStarted'),
      meta: { headerClassName: 'w-44' },
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
      meta: { headerClassName: 'w-28' },
      cell: ({ row }) => (
        <FileStatusBadge
          status={row.original.status as RunningStatus}
          name={RunningStatusMap[row.original.status as RunningStatus]}
          className="!w-20"
        />
      ),
    },
  ];

  if (showDatasetColumn) {
    columns.push({
      accessorKey: 'kb_name',
      header: t('knowledgeDetails.dataset'),
      meta: { headerClassName: 'w-1/4' },
      cell: ({ row }) => (
        <div
          className="flex items-center gap-2 text-text-primary cursor-pointer"
          onClick={() => {
            handleToDataSetDetail?.(row.original.kb_id);
          }}
        >
          <RAGFlowAvatar
            avatar={row.original.avatar}
            name={row.original.kb_name}
            className="size-4"
          />
          <span className="truncate">{row.original.kb_name}</span>
        </div>
      ),
    });
  }

  columns.push(
    {
      accessorKey: 'task_type',
      header: 'Task Type',
      meta: { headerClassName: 'w-28' },
      cell: ({ row }) => row.original.task_type || 'sync',
    },
    {
      id: 'summary',
      header: 'Summary',
      cell: ({ row }) => (
        <div className="max-w-[32rem] whitespace-normal break-words text-text-primary">
          {getSummary(row.original, now)}
        </div>
      ),
    },
  );

  return columns;
};
