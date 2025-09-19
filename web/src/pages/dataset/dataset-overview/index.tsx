import SvgIcon from '@/components/svg-icon';
import { useIsDarkTheme } from '@/components/theme-provider';
import { parseColorToRGBA } from '@/utils/common-util';
import { CircleQuestionMark } from 'lucide-react';
import { FC, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LogTabs } from './dataset-common';
import { DatasetFilter } from './dataset-filter';
import { useFetchOverviewTital } from './hook';
import FileLogsTable from './overview-table';

interface StatCardProps {
  title: string;
  value: number;
  icon: JSX.Element;
  children?: JSX.Element;
}

const StatCard: FC<StatCardProps> = ({ title, value, children, icon }) => {
  return (
    <div className="bg-bg-card  p-4 rounded-lg border border-border flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <h3 className="flex items-center gap-1 text-sm font-medium text-text-secondary">
          {title}
          <CircleQuestionMark size={12} />
        </h3>
        {icon}
      </div>
      <div className="text-2xl font-bold text-text-primary">{value}</div>
      <div className="h-12 w-full flex items-center">
        <div className="flex-1">{children}</div>
      </div>
    </div>
  );
};

interface CardFooterProcessProps {
  success: number;
  failed: number;
}
const CardFooterProcess: FC<CardFooterProcessProps> = ({
  success = 0,
  failed = 0,
}) => {
  const { t } = useTranslation();
  return (
    <div className="flex items-center flex-col gap-2">
      <div className="w-full flex justify-between gap-4 rounded-lg text-sm font-bold text-text-primary">
        <div
          className="flex items-center justify-between  rounded-md w-1/2 p-2"
          style={{
            backgroundColor: `${parseColorToRGBA('var(--state-success)', 0.05)}`,
          }}
        >
          <div className="flex items-center rounded-lg gap-1">
            <div className="w-2 h-2 rounded-full bg-state-success"></div>
            <div>{t('knowledgeDetails.success')}</div>
          </div>
          <div>{success || 0}</div>
        </div>
        <div className="flex items-center justify-between rounded-md w-1/2 bg-state-error-5 p-2">
          <div className="flex items-center rounded-lg gap-1">
            <div className="w-2 h-2 rounded-full bg-state-error"></div>
            <div>{t('knowledgeDetails.failed')}</div>
          </div>
          <div>{failed || 0}</div>
        </div>
      </div>
    </div>
  );
};
const FileLogsPage: FC = () => {
  const { t } = useTranslation();
  const [active, setActive] = useState<(typeof LogTabs)[keyof typeof LogTabs]>(
    LogTabs.FILE_LOGS,
  );
  const topMockData = {
    totalFiles: {
      value: 2827,
      precent: 12.5,
    },
    downloads: {
      value: 28,
      success: 8,
      failed: 2,
    },
    processing: {
      value: 156,
      success: 8,
      failed: 2,
    },
  };

  const { data: topData } = useFetchOverviewTital();
  console.log('topData --> ', topData);

  const mockData = useMemo(() => {
    if (active === LogTabs.FILE_LOGS) {
      return Array(30)
        .fill(0)
        .map((_, i) => ({
          id: i === 0 ? '952734' : `14`,
          fileName: 'PRD for DealBees 1.2 (1).txt',
          source: 'GitHub',
          pipeline:
            i === 0 ? 'data demo for...' : i === 1 ? 'test' : 'kiki’s demo',
          startDate: '14/03/2025 14:53:39',
          task: i === 0 ? 'chunck' : 'Parser',
          status: i === 0 ? 3 : i === 1 ? 4 : i === 2 ? 1 : 0,
          statusName:
            i === 0
              ? 'Success'
              : i === 1
                ? 'Failed'
                : i === 2
                  ? 'Running'
                  : 'Pending',
        }));
    }
    if (active === LogTabs.DATASET_LOGS) {
      return Array(8)
        .fill(0)
        .map((_, i) => ({
          id: i === 0 ? '952734' : `14`,
          fileName: 'PRD for DealBees 1.2 (1).txt',
          source: 'GitHub',
          startDate: '14/03/2025 14:53:39',
          task: i === 0 ? 'chunck' : 'Parser',
          pipeline:
            i === 0 ? 'data demo for...' : i === 1 ? 'test' : 'kiki’s demo',
          status: i === 0 ? 3 : i === 1 ? 4 : i === 2 ? 1 : 0,
          statusName:
            i === 0
              ? 'Success'
              : i === 1
                ? 'Failed'
                : i === 2
                  ? 'Running'
                  : 'Pending',
        }));
    }
  }, [active]);

  const pagination = {
    current: 1,
    pageSize: 10,
    total: 100,
  };

  const changeActiveLogs = (active: (typeof LogTabs)[keyof typeof LogTabs]) => {
    setActive(active);
  };
  const handlePaginationChange = (page: number, pageSize: number) => {
    console.log('Pagination changed:', { page, pageSize });
  };

  const isDark = useIsDarkTheme();
  return (
    <div className="p-5 min-w-[880px] border-border border rounded-lg mr-5">
      {/* Stats Cards */}
      <div className="grid grid-cols-3 md:grid-cols-3 gap-4 mb-6">
        <StatCard
          title="Total Files"
          value={topMockData.totalFiles.value}
          icon={
            isDark ? (
              <SvgIcon name="data-flow/total-files-icon" width={40} />
            ) : (
              <SvgIcon name="data-flow/total-files-icon-bri" width={40} />
            )
          }
        >
          <div>
            <span className="text-accent-primary">
              {topMockData.totalFiles.precent > 0 ? '+' : ''}
              {topMockData.totalFiles.precent}%{' '}
            </span>
            from last week
          </div>
        </StatCard>
        <StatCard
          title="Downloading"
          value={topMockData.downloads.value}
          icon={
            isDark ? (
              <SvgIcon name="data-flow/data-icon" width={40} />
            ) : (
              <SvgIcon name="data-flow/data-icon-bri" width={40} />
            )
          }
        >
          <CardFooterProcess
            success={topMockData.downloads.success}
            failed={topMockData.downloads.failed}
          />
        </StatCard>
        <StatCard
          title="Processing"
          value={topMockData.processing.value}
          icon={
            isDark ? (
              <SvgIcon name="data-flow/processing-icon" width={40} />
            ) : (
              <SvgIcon name="data-flow/processing-icon-bri" width={40} />
            )
          }
        >
          <CardFooterProcess
            success={topMockData.processing.success}
            failed={topMockData.processing.failed}
          />
        </StatCard>
      </div>

      {/* Tabs & Search */}
      <DatasetFilter active={active} setActive={changeActiveLogs} />

      {/* Table */}
      <FileLogsTable
        data={mockData}
        pagination={pagination}
        setPagination={handlePaginationChange}
        pageCount={10}
        active={active}
      />
    </div>
  );
};

export default FileLogsPage;
