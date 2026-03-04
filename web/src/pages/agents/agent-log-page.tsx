import TimeRangePicker from '@/components/originui/time-range-picker';
import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { SearchInput } from '@/components/ui/input';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { Spin } from '@/components/ui/spin';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentLog } from '@/hooks/use-agent-request';
import {
  IAgentLogMessage,
  IAgentLogResponse,
} from '@/interfaces/database/agent';
import { IReferenceObject } from '@/interfaces/database/chat';
import { useQueryClient } from '@tanstack/react-query';
import React, { useEffect, useState } from 'react';
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
  const { navigateToAgents, navigateToAgent } = useNavigatePage();
  const { flowDetail: agentDetail } = useFetchDataOnMount();
  const { id: canvasId } = useParams();
  const queryClient = useQueryClient();
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
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: 'Title',
      dataIndex: 'title',
      key: 'title',
      render: (text, record: IAgentLogResponse) => (
        <span>
          {record?.message?.length ? record?.message[0]?.content : ''}
        </span>
      ),
    },
    {
      title: 'State',
      dataIndex: 'state',
      key: 'state',
      render: (text, record: IAgentLogResponse) => (
        <div
          className="size-2 rounded-full"
          style={{ backgroundColor: record.errors ? 'red' : 'green' }}
        ></div>
      ),
    },
    {
      title: 'Number',
      dataIndex: 'round',
      key: 'round',
    },
    {
      title: 'Latest Date',
      dataIndex: 'update_date',
      key: 'update_date',
      sortable: true,
    },
    {
      title: 'Create Date',
      dataIndex: 'create_date',
      key: 'create_date',
      sortable: true,
    },
  ];

  const { data: logData, loading } = useFetchAgentLog(searchParams);
  const { sessions: data, total } = logData || {};
  const [currentDate, setCurrentDate] = useState<DateRange>({
    from: searchParams.from_date,
    to: searchParams.to_date,
  });
  const [keywords, setKeywords] = useState(searchParams.keywords);
  const handleDateRangeChange = ({
    from: startDate,
    to: endDate,
  }: DateRange) => {
    setCurrentDate({ from: startDate, to: endDate });
  };

  const [pagination, setPagination] = useState<{
    current: number;
    pageSize: number;
    total: number;
  }>({
    current: 1,
    pageSize: 10,
    total: total,
  });

  useEffect(() => {
    setPagination((pre) => {
      return {
        ...pre,
        total: total,
      };
    });
  }, [total]);

  const [sortConfig, setSortConfig] = useState<{
    orderby: string;
    desc: boolean;
  } | null>({ orderby: init.orderby, desc: init.desc ? true : false });

  const handlePageChange = (current?: number, pageSize?: number) => {
    console.log('current', current, 'pageSize', pageSize);
    let page = current || 1;
    if (pagination.pageSize !== pageSize) {
      page = 1;
    }
    setPagination({
      ...pagination,
      current: page,
      pageSize: pageSize || 10,
    });
  };

  const handleSearch = () => {
    setSearchParams((pre) => {
      return {
        ...pre,
        from_date: currentDate.from as Date,
        to_date: currentDate.to as Date,
        page: pagination.current,
        page_size: pagination.pageSize,
        orderby: sortConfig?.orderby || '',
        desc: sortConfig?.desc as boolean,
        keywords: keywords,
      };
    });
  };

  const handleClickSearch = () => {
    setPagination({ ...pagination, current: 1 });
    handleSearch();
    queryClient.invalidateQueries({
      queryKey: ['fetchAgentLog'],
    });
  };
  useEffect(() => {
    handleSearch();
  }, [pagination.current, pagination.pageSize, sortConfig]);
  // handle sort
  const handleSort = (key: string) => {
    let desc = false;
    if (sortConfig && sortConfig.orderby === key) {
      desc = !sortConfig.desc;
    }
    setSortConfig({ orderby: key, desc });
  };

  const handleReset = () => {
    setSearchParams(init);
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

  return (
    <div className=" text-white">
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToAgents}>Agent</BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToAgent(canvasId as string)}>
                {agentDetail.title}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>Log</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      <div className="p-4">
        <div className="flex justify-between items-center">
          <h1 className="text-2xl font-bold mb-4">Log</h1>

          <div className="flex justify-end space-x-2 mb-4 text-foreground">
            <div className="flex items-center space-x-2">
              <span>ID/Title</span>
              <SearchInput
                value={keywords}
                onChange={(e) => {
                  setKeywords(e.target.value);
                }}
                className="w-32"
              ></SearchInput>
            </div>
            <div className="flex items-center space-x-2">
              <span className="whitespace-nowrap">Latest Date</span>
              <TimeRangePicker
                onSelect={handleDateRangeChange}
                selectDateRange={currentDate}
              />
            </div>
            <button
              type="button"
              className="bg-foreground  text-text-title-invert  px-4 py-1 rounded"
              onClick={() => {
                handleClickSearch();
              }}
            >
              Search
            </button>
            <button
              type="button"
              className="bg-transparent text-foreground px-4 py-1 rounded border"
              onClick={() => handleReset()}
            >
              Reset
            </button>
          </div>
        </div>
        <div className="border rounded-md overflow-auto">
          {/* <div className="max-h-[500px] overflow-y-auto w-full"> */}
          <Table rootClassName="max-h-[calc(100vh-200px)]">
            <TableHeader className="sticky top-0 bg-background z-10 shadow-sm">
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
                        sortConfig?.orderby === column.dataIndex && (
                          <span className="ml-1">
                            {sortConfig.desc ? '↓' : '↑'}
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
                          ? column.render(item[column.dataIndex], item)
                          : item[column.dataIndex]}
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
                    No data
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
              {...pagination}
              total={pagination.total}
              onChange={(page, pageSize) => {
                handlePageChange(page, pageSize);
              }}
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
