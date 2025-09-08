import {
  CircleQuestionMark,
  Cpu,
  FileChartLine,
  HardDriveDownload,
} from 'lucide-react';
import { FC, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LogTabs } from './dataset-common';
import { DatasetFilter } from './dataset-filter';
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
  total: number;
  completed: number;
  success: number;
  failed: number;
}
const CardFooterProcess: FC<CardFooterProcessProps> = ({
  total,
  completed,
  success,
  failed,
}) => {
  const { t } = useTranslation();
  const successPrecentage = (success / total) * 100;
  const failedPrecentage = (failed / total) * 100;
  return (
    <div className="flex items-center flex-col gap-2">
      <div className="flex justify-between w-full text-sm text-text-secondary">
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-1">
            {success}
            <span>{t('knowledgeDetails.success')}</span>
          </div>
          <div className="flex items-center gap-1">
            {failed}
            <span>{t('knowledgeDetails.failed')}</span>
          </div>
        </div>
        <div className="flex items-center gap-1">
          {completed}
          <span>{t('knowledgeDetails.completed')}</span>
        </div>
      </div>
      <div className="w-full flex rounded-full h-3 bg-bg-card text-sm font-bold text-text-primary">
        <div
          className=" rounded-full h-3 bg-accent-primary"
          style={{ width: successPrecentage + '%' }}
        ></div>
        <div
          className=" rounded-full h-3 bg-state-error"
          style={{ width: failedPrecentage + '%' }}
        ></div>
      </div>
    </div>
  );
};
const FileLogsPage: FC = () => {
  const { t } = useTranslation();
  const [active, setActive] = useState<(typeof LogTabs)[keyof typeof LogTabs]>(
    LogTabs.FILE_LOGS,
  );
  const mockData = Array(30)
    .fill(0)
    .map((_, i) => ({
      id: i === 0 ? '#952734' : `14`,
      fileName: 'PRD for DealBees 1.2 (1).txt',
      source: 'GitHub',
      pipeline: i === 0 ? 'data demo for...' : i === 1 ? 'test' : 'kiki’s demo',
      startDate: '14/03/2025 14:53:39',
      task: i === 0 ? 'Parse' : 'Parser',
      status:
        i === 0
          ? 'Success'
          : i === 1
            ? 'Failed'
            : i === 2
              ? 'Running'
              : 'Pending',
    }));

  const pagination = {
    current: 1,
    pageSize: 30,
    total: 100,
  };

  const changeActiveLogs = (active: (typeof LogTabs)[keyof typeof LogTabs]) => {
    setActive(active);
  };
  const handlePaginationChange = (page: number, pageSize: number) => {
    console.log('Pagination changed:', { page, pageSize });
  };

  return (
    <div className="p-5 min-w-[880px] border-border border rounded-lg mr-5">
      {/* Stats Cards */}
      <div className="grid grid-cols-3 md:grid-cols-3 gap-4 mb-6">
        <StatCard title="Total Files" value={2827} icon={<FileChartLine />}>
          <div>+7% from last week</div>
        </StatCard>
        <StatCard title="Downloading" value={28} icon={<HardDriveDownload />}>
          <CardFooterProcess
            total={100}
            success={8}
            failed={2}
            completed={15}
          />
        </StatCard>
        <StatCard title="Processing" value={156} icon={<Cpu />}>
          <CardFooterProcess total={20} success={8} failed={2} completed={15} />
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
