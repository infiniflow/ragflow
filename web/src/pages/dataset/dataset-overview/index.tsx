import { FilterCollection } from '@/components/list-filter-bar/interface';
import SvgIcon from '@/components/svg-icon';
import { useIsDarkTheme } from '@/components/theme-provider';

import {
  Card,
  CardDescription,
  CardFooter,
  CardHeader,
} from '@/components/ui/card';

import WhatIsThis from '@/components/what-is-this';
import { RunningStatusMap } from '@/constants/knowledge';
import { useFetchDocumentList } from '@/hooks/use-document-request';
import { FC, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RunningStatus } from '../dataset/constant';
import { LogTabs } from './dataset-common';
import { DatasetFilter } from './dataset-filter';
import { useFetchFileLogList, useFetchOverviewTital } from './hook';
import { DocumentLog, IFileLogItem } from './interface';
import FileLogsTable from './overview-table';

interface StatCardProps {
  title: string;
  value: number;
  icon: JSX.Element;
  children?: JSX.Element;
  tooltip?: string;
}
interface CardFooterProcessProps {
  success: number;
  failed: number;
  successTip?: string;
  failedTip?: string;
}

const StatCard: FC<StatCardProps> = ({
  title,
  value,
  children,
  icon,
  tooltip,
}) => {
  return (
    <Card
      className="px-5 py-2.5 rounded-lg border-border-default grid grid-cols-[1fr_auto] grid-rows-[1fr_auto]"
      style={{
        gridTemplateAreas: '"data icon" "footer footer"',
      }}
    >
      <span style={{ gridArea: 'icon' }}>{icon}</span>

      <div style={{ gridArea: 'data' }}>
        <CardHeader className="p-0">
          <h3 className="flex items-center gap-1 text-sm font-medium text-text-secondary">
            {title}

            {tooltip && <WhatIsThis>{tooltip}</WhatIsThis>}
          </h3>
        </CardHeader>

        <CardDescription className="text-text-primary text-2xl font-medium leading-9">
          <data value={value}>{value}</data>
        </CardDescription>
      </div>

      <CardFooter
        className="p-0 mt-1.5 h-8 w-full flex items-end"
        style={{ gridArea: 'footer' }}
      >
        <div className="flex-1">{children}</div>
      </CardFooter>
    </Card>
  );
};

const CardFooterProcess: FC<CardFooterProcessProps> = ({
  success = 0,
  successTip,
  failed = 0,
  failedTip,
}) => {
  const { t } = useTranslation();

  return (
    <div className="flex items-center flex-col gap-2">
      <dl className="w-full flex justify-between gap-4 rounded-lg text-sm font-bold text-text-primary">
        <div className="flex items-center justify-between rounded-sm w-1/2 p-2 bg-state-success-5">
          <dt className="flex items-center rounded-lg gap-1">
            <div className="w-1 h-1 rounded-full bg-state-success"></div>
            <div className="font-normal text-text-secondary text-xs flex items-center gap-1">
              {t('knowledgeDetails.success')}
              {successTip && <WhatIsThis>{successTip}</WhatIsThis>}
            </div>
          </dt>

          <dd className="font-normal">{success || 0}</dd>
        </div>

        <div className="flex items-center justify-between rounded-sm w-1/2 bg-state-error-5 p-2">
          <dt className="flex items-center rounded-lg gap-1">
            <div className="w-1 h-1 rounded-full bg-state-error"></div>
            <div className="font-normal text-text-secondary text-xs flex items-center gap-1">
              {t('knowledgeDetails.failed')}
              {failedTip && <WhatIsThis>{failedTip}</WhatIsThis>}
            </div>
          </dt>

          <dd className="font-normal">{failed || 0}</dd>
        </div>
      </dl>
    </div>
  );
};

const FileLogsPage: FC = () => {
  const { t } = useTranslation();

  const [topAllData, setTopAllData] = useState({
    totalFiles: {
      value: 0,
      precent: 0,
    },
    downloads: {
      value: 0,
      success: 0,
      failed: 0,
    },
    processing: {
      value: 0,
      success: 0,
      failed: 0,
    },
  });
  const { data: topData } = useFetchOverviewTital();
  const {
    pagination: { total: fileTotal },
  } = useFetchDocumentList(false);

  useEffect(() => {
    setTopAllData((prev) => {
      return {
        ...prev,
        downloads: {
          ...prev.downloads,
          success: topData?.downloaded || 0,
        },
        processing: {
          value: topData?.processing || 0,
          success: topData?.finished || 0,
          failed: topData?.failed || 0,
        },
      };
    });
  }, [topData]);

  useEffect(() => {
    setTopAllData((prev) => {
      return {
        ...prev,
        totalFiles: {
          value: fileTotal || 0,
          precent: 0,
        },
      };
    });
  }, [fileTotal]);

  const {
    data: tableOriginData,
    searchString,
    handleInputChange,
    pagination,
    setPagination,
    active,
    filterValue,
    setFilterValue,
    handleFilterSubmit,
    setActive,
  } = useFetchFileLogList();

  const filters = useMemo(() => {
    const filterCollection: FilterCollection[] = [
      {
        field: 'operation_status',
        label: t('knowledgeDetails.status'),
        list: Object.values(RunningStatus).map((value) => {
          // const value = key as RunningStatus;
          return {
            id: value,
            // label: RunningStatusMap[value].label,
            label: RunningStatusMap[value],
          };
        }),
      },
      // {
      //   field: 'types',
      //   label: t('knowledgeDetails.task'),
      //   list: [
      //     {
      //       id: 'Parse',
      //       label: 'Parse',
      //     },
      //     {
      //       id: 'Download',
      //       label: 'Download',
      //     },
      //   ],
      // },
    ];
    if (active === LogTabs.FILE_LOGS) {
      return filterCollection;
    }
    if (active === LogTabs.DATASET_LOGS) {
      const list = filterCollection.filter((item, index) => index === 0);
      return list;
    }
    return [];
  }, [active, t]);

  const tableList = useMemo(() => {
    console.log('tableList', tableOriginData);
    if (tableOriginData && tableOriginData.logs?.length) {
      return tableOriginData.logs.map((item) => {
        return {
          ...item,
          status: item.operation_status as RunningStatus,
          statusName: RunningStatusMap[item.operation_status as RunningStatus],
        } as unknown as IFileLogItem & DocumentLog;
      });
    }
    return [];
  }, [tableOriginData]);

  const changeActiveLogs = (active: (typeof LogTabs)[keyof typeof LogTabs]) => {
    setFilterValue({});
    setActive(active);
  };
  const handlePaginationChange = ({
    page,
    pageSize,
  }: {
    page: number;
    pageSize: number;
  }) => {
    setPagination({
      ...pagination,
      page,
      pageSize: pageSize,
    });
  };

  const isDark = useIsDarkTheme();

  return (
    <Card
      className="
      p-5 min-w-[880px] mr-5 mb-5 bg-transparent shadow-none
      flex flex-col overflow-y-auto scrollbar-auto"
    >
      {/* Stats Cards */}
      <div className="grid grid-cols-3 md:grid-cols-3 gap-7 mb-6">
        <StatCard
          title={t('datasetOverview.totalFiles')}
          value={topAllData.totalFiles.value}
          icon={
            isDark ? (
              <SvgIcon name="data-flow/total-files-icon" width={40} />
            ) : (
              <SvgIcon name="data-flow/total-files-icon-bri" width={40} />
            )
          }
        >
          <div className="text-xs">
            <span className="text-accent-primary">
              {topAllData.totalFiles.precent > 0 ? '+' : ''}
              {topAllData.totalFiles.precent}%{' '}
            </span>
            <span className="font-normal text-text-secondary">
              {t('knowledgeConfiguration.lastWeek')}
            </span>
          </div>
        </StatCard>
        <StatCard
          title={t('datasetOverview.downloading')}
          value={topAllData.downloads.value}
          icon={
            isDark ? (
              <SvgIcon name="data-flow/data-icon" width={40} />
            ) : (
              <SvgIcon name="data-flow/data-icon-bri" width={40} />
            )
          }
          tooltip={t('datasetOverview.downloadTip')}
        >
          <CardFooterProcess
            success={topAllData.downloads.success}
            successTip={t('datasetOverview.downloadSuccessTip')}
            failed={topAllData.downloads.failed}
            failedTip={t('datasetOverview.downloadFailedTip')}
          />
        </StatCard>
        <StatCard
          title={t('datasetOverview.processing')}
          value={topAllData.processing.value}
          icon={
            isDark ? (
              <SvgIcon name="data-flow/processing-icon" width={40} />
            ) : (
              <SvgIcon name="data-flow/processing-icon-bri" width={40} />
            )
          }
          tooltip={t('datasetOverview.processingTip')}
        >
          <CardFooterProcess
            success={topAllData.processing.success}
            successTip={t('datasetOverview.processingSuccessTip')}
            failed={topAllData.processing.failed}
            failedTip={t('datasetOverview.processingFailedTip')}
          />
        </StatCard>
      </div>

      {/* Tabs & Search */}
      <DatasetFilter
        filters={filters as FilterCollection[]}
        value={filterValue}
        active={active}
        setActive={changeActiveLogs}
        searchString={searchString}
        onSearchChange={handleInputChange}
        onChange={handleFilterSubmit}
      />

      {/* Table */}
      <FileLogsTable
        data={tableList}
        pagination={pagination}
        setPagination={handlePaginationChange}
        pageCount={10}
        active={active}
      />
    </Card>
  );
};

export default FileLogsPage;
