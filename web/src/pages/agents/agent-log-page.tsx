import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { DatePickerWithRange } from '@/components/ui/range-picker';
import { Spin } from '@/components/ui/spin';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentLog } from '@/hooks/use-agent-request';
import {
  IAgentLogMessage,
  IAgentLogResponse,
} from '@/interfaces/database/agent';
import { IReferenceObject } from '@/interfaces/database/chat';
import { formatDate } from '@/utils/date';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { DateRange } from '../../components/originui/calendar/index';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../components/ui/table';
import { useFetchDataOnMount } from '../agent/hooks/use-fetch-data';
import { AgentLogDetailModal } from './agent-log-detail-modal';
import { useExportAgentLogToCSV } from './hooks/use-export-agent-log';
const getStartOfToday = (): Date => {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return today;
};

const getEndOfToday = (): Date => {
  const today = new Date();
  today.setHours(23, 59, 59, 999);
  return today;
};

const AgentLogPage: React.FC = () => {
  const { t } = useTranslation();
  const { navigateToAgents, navigateToAgent } = useNavigatePage();
  const { flowDetail: agentDetail } = useFetchDataOnMount();
  const { id: canvasId } = useParams();
  const init = {
    keywords: '',
    from_date: getStartOfToday(),
    to_date: getEndOfToday(),
    orderby: 'create_time',
    desc: false,
    page: 1,
    page_size: 10,
  };
  const [searchParams, setSearchParams] = useState(init);

  const columns = [
    {
      title: t('flow.id'),
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: t('flow.userId'),
      dataIndex: 'user_id',
      key: 'user_id',
      render: (text: string) => <span>{text}</span>,
    },
    {
      title: t('flow.logTitle'),
      dataIndex: 'title',
      key: 'title',
      render: (_text: string, record: IAgentLogResponse) => (
        <span>
          {record?.message?.length ? record?.message[0]?.content : ''}
        </span>
      ),
    },
    {
      title: t('flow.state'),
      dataIndex: 'state',
      key: 'state',
      render: (_text: string, record: IAgentLogResponse) => (
        <div
          className="size-2 rounded-full"
          style={{ backgroundColor: record.errors ? 'red' : 'green' }}
        ></div>
      ),
    },
    {
      title: t('flow.number'),
      dataIndex: 'round',
      key: 'round',
    },
    {
      title: t('flow.latestDate'),
      dataIndex: 'update_date',
      key: 'update_date',
      sortable: true,
      render(text: string) {
        return formatDate(text);
      },
    },
    {
      title: t('flow.createDate'),
      dataIndex: 'create_date',
      key: 'create_date',
      sortable: true,
      render(text: string) {
        return formatDate(text);
      },
    },
    {
      title: t('flow.version.version'),
      dataIndex: 'version_title',
      key: 'version_title',
    },
  ];

  const { data: logData, loading, refetch } = useFetchAgentLog(searchParams);
  const { sessions: data, total } = logData || {};
  const { handleExport, loading: exportLoading } = useExportAgentLogToCSV();
  const [currentDate, setCurrentDate] = useState<DateRange>({
    from: searchParams.from_date,
    to: searchParams.to_date,
  });
  const [keywords, setKeywords] = useState(searchParams.keywords);
  const handleDateRangeChange = (dateRange: DateRange) => {
    setCurrentDate({ from: dateRange.from, to: dateRange.to });
  };

  const handlePageChange = (current: number, pageSize: number) => {
    setSearchParams((pre) => ({
      ...pre,
      page: pre.page_size === pageSize ? current : 1,
      page_size: pageSize,
    }));
  };

  const handleClickSearch = () => {
    const sameParams =
      searchParams.page === 1 &&
      searchParams.keywords === keywords &&
      searchParams.from_date?.getTime() === currentDate.from?.getTime() &&
      searchParams.to_date?.getTime() === currentDate.to?.getTime();

    if (sameParams) {
      refetch();
    } else {
      setSearchParams((pre) => ({
        ...pre,
        from_date: currentDate.from as Date,
        to_date: currentDate.to as Date,
        page: 1,
        keywords,
      }));
    }
  };

  const handleSort = (key: string) => {
    setSearchParams((pre) => ({
      ...pre,
      orderby: key,
      desc: pre.orderby === key ? !pre.desc : false,
    }));
  };

  const handleReset = () => {
    setSearchParams({ ...init, page_size: searchParams.page_size });
    setKeywords(init.keywords);
    setCurrentDate({ from: init.from_date, to: init.to_date });
  };

  const [openModal, setOpenModal] = useState(false);
  const [modalData, setModalData] = useState<IAgentLogResponse>();
  const showLogDetail = (item: IAgentLogResponse) => {
    if (item?.round) {
      setModalData(item);
      setOpenModal(true);
    }
  };

  const onExportClick = () => {
    handleExport({
      keywords: searchParams.keywords,
      from_date: searchParams.from_date,
      to_date: searchParams.to_date,
      orderby: searchParams.orderby,
      desc: searchParams.desc,
      page: searchParams.page,
      page_size: searchParams.page_size,
    });
  };

  return (
    <div className=" text-text-primary">
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToAgents}>
                {t('flow.agent')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToAgent(canvasId as string)}>
                {agentDetail.title}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{t('flow.log')}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      <div className="p-4">
        <div className="flex justify-between items-center">
          <h1 className="text-2xl font-bold mb-4">{t('flow.log')}</h1>

          <div className="flex justify-end space-x-2 mb-4 text-foreground">
            <div className="flex items-center space-x-2">
              <Button onClick={onExportClick} loading={exportLoading}>
                {t('flow.exportCurrentPage')}
              </Button>
              <span>{`${t('flow.id')}/${t('flow.logTitle')}`}</span>
              <SearchInput
                value={keywords}
                onChange={(e) => {
                  setKeywords(e.target.value);
                }}
                className="w-32"
              ></SearchInput>
            </div>
            <div className="flex items-center space-x-2">
              <span className="whitespace-nowrap">{t('flow.latestDate')}</span>
              <DatePickerWithRange
                required
                selected={currentDate}
                onSelect={(range) =>
                  range.from &&
                  handleDateRangeChange({ from: range.from, to: range.to })
                }
              ></DatePickerWithRange>
            </div>
            <button
              type="button"
              className="bg-foreground  text-text-title-invert  px-4 py-1 rounded"
              onClick={handleClickSearch}
            >
              {t('common.search')}
            </button>
            <button
              type="button"
              className="bg-transparent text-foreground px-4 py-1 rounded border"
              onClick={handleReset}
            >
              {t('common.reset')}
            </button>
          </div>
        </div>
        <div className="border rounded-md overflow-auto">
          {/* <div className="max-h-[500px] overflow-y-auto w-full"> */}
          <Table rootClassName="max-h-[calc(100vh-200px)]">
            <TableHeader className="sticky top-0 bg-bg-title z-10 shadow-sm">
              <TableRow>
                {columns.map((column) => (
                  <TableHead
                    key={column.dataIndex}
                    onClick={
                      column.sortable
                        ? () => handleSort(column.dataIndex)
                        : undefined
                    }
                    className={
                      column.sortable ? 'cursor-pointer hover:bg-muted/50' : ''
                    }
                  >
                    <div className="flex items-center">
                      {column.title}
                      {column.sortable &&
                        searchParams.orderby === column.dataIndex && (
                          <span className="ml-1">
                            {searchParams.desc ? '↓' : '↑'}
                          </span>
                        )}
                    </div>
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && (
                <TableRow>
                  <TableCell
                    colSpan={columns.length}
                    className="h-24 text-center"
                  >
                    <Spin size="large">
                      <span className="sr-only">Loading...</span>
                    </Spin>
                  </TableCell>
                </TableRow>
              )}
              {!loading &&
                data?.map((item) => (
                  <TableRow
                    key={item.id}
                    onClick={() => {
                      showLogDetail(item);
                    }}
                  >
                    {columns.map((column) => (
                      <TableCell key={column.dataIndex}>
                        {column.render
                          ? column.render(
                              item[column.dataIndex as keyof IAgentLogResponse],
                              item,
                            )
                          : (item[
                              column.dataIndex as keyof typeof item
                            ] as string)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              {!loading && (!data || data.length === 0) && (
                <TableRow>
                  <TableCell
                    colSpan={columns.length}
                    className="h-24 text-center"
                  >
                    {t('common.noData')}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          {/* </div> */}
        </div>
        <div className="flex justify-end mt-4 w-full">
          <div className="space-x-2">
            <RAGFlowPagination
              current={searchParams.page}
              pageSize={searchParams.page_size}
              total={total}
              onChange={handlePageChange}
            ></RAGFlowPagination>
          </div>
        </div>
      </div>
      <AgentLogDetailModal
        isOpen={openModal}
        message={modalData?.message as IAgentLogMessage[]}
        reference={modalData?.reference as unknown as IReferenceObject}
        onClose={() => setOpenModal(false)}
      />
    </div>
  );
};

export default AgentLogPage;
