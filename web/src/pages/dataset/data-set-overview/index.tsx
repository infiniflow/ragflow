import { FC } from 'react';
import { useTranslation } from 'react-i18next';
import { DatasetFilter } from './dataset-filter';
import FileLogsTable from './overview-table';

interface StatCardProps {
  title: string;
  value: number;
  change?: string;
  icon: JSX.Element;
}

const StatCard: FC<StatCardProps> = ({ title, value, change, icon }) => {
  return (
    <div className="bg-bg-card  p-4 rounded-lg border border-border flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-text-secondary">{title}</h3>
        {icon}
      </div>
      <div className="text-2xl font-bold text-text-primary">{value}</div>
      {change && <div className="text-xs text-green-400">{change}</div>}
    </div>
  );
};

const FileLogsPage: FC = () => {
  const { t } = useTranslation();

  const mockData = Array(30)
    .fill(0)
    .map((_, i) => ({
      id: i === 0 ? '#952734' : `14`,
      fileName: 'PRD for DealBees 1.2 (1).text',
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

  const handlePaginationChange = (page: number, pageSize: number) => {
    console.log('Pagination changed:', { page, pageSize });
  };

  return (
    <div className="p-5 min-w-[880px]">
      {/* Stats Cards */}
      <div className="grid grid-cols-3 md:grid-cols-3 gap-4 mb-6">
        <StatCard
          title="Total Files"
          value={2827}
          change="+7% from last week"
          icon={
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M14 2H6a2 2 0 0 0-2 2v16h2"></path>
              <path d="M14 2v4a2 2 0 0 0 2 2h4"></path>
            </svg>
          }
        />
        <StatCard
          title="Downloading"
          value={28}
          change="18 success, 2 failed"
          icon={
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
              <polyline points="7 10 12 15 17 10"></polyline>
              <line x1="12" y1="15" x2="12" y2="3"></line>
            </svg>
          }
        />
        <StatCard
          title="Processing"
          value={156}
          change="18 success, 2 failed"
          icon={
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect>
              <line x1="8" y1="21" x2="16" y2="21"></line>
              <line x1="12" y1="17" x2="12" y2="21"></line>
            </svg>
          }
        />
      </div>

      {/* Tabs & Search */}
      <DatasetFilter />

      {/* Table */}
      <FileLogsTable
        data={mockData}
        pagination={pagination}
        setPagination={handlePaginationChange}
        pageCount={10}
      />
    </div>
  );
};

export default FileLogsPage;
