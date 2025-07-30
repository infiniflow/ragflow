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
import { IAgentLogResponse } from '@/interfaces/database/agent';
import React, { useEffect, useState } from 'react';
import { useParams } from 'umi';
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

const AgentLogPage: React.FC = () => {
  const { navigateToAgentList, navigateToAgent } = useNavigatePage();
  const { flowDetail: agentDetail } = useFetchDataOnMount();
  const { id: canvasId } = useParams();
  const today = new Date();
  const init = {
    keywords: '',
    from_date: today,
    to_date: today,
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
    setPagination((pre) => {
      return {
        ...pre,
        current: current ?? pre.current,
        pageSize: pageSize ?? pre.pageSize,
      };
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
  };
  return (
    <div className=" text-white">
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToAgentList}>
                Agent
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
              <BreadcrumbPage>Log</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      <div className="p-4">
        <div className="flex justify-between items-center">
          <h1 className="text-2xl font-bold mb-4">Log</h1>

          <div className="flex justify-end space-x-2 mb-4">
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
                selectDateRange={{ from: currentDate.from, to: currentDate.to }}
              />
            </div>
            <button
              type="button"
              className="bg-foreground  text-text-title-invert  px-4 py-1 rounded"
              onClick={() => {
                setPagination({ ...pagination, current: 1 });
                handleSearch();
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
                  <TableRow key={item.id}>
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
    </div>
  );
};

export default AgentLogPage;
