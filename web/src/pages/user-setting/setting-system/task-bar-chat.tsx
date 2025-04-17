import { TaskExecutorHeartbeatItem } from '@/interfaces/database/user-setting';
import { Button, Divider, Flex, Popconfirm, Tooltip } from 'antd';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Tooltip as ChartTooltip,
  Legend,
  Rectangle,
  ResponsiveContainer,
  XAxis,
} from 'recharts';

import { useDeleteTaskExecutor } from '@/hooks/user-setting-hooks';
import { formatDate, formatTime } from '@/utils/date';
import { DeleteOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { get } from 'lodash';
import { useTranslation } from 'react-i18next';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';

import styles from './index.less';

interface IProps {
  data: Record<string, TaskExecutorHeartbeatItem[]>;
}

const CustomTooltip = ({ active, payload, ...restProps }: any) => {
  if (active && payload && payload.length) {
    const taskExecutorHeartbeatItem: TaskExecutorHeartbeatItem = get(
      payload,
      '0.payload',
      {},
    );
    return (
      <div className="custom-tooltip">
        <div className="bg-slate-50 p-2 rounded-md border border-indigo-100">
          <div className="font-semibold text-lg">
            {formatDate(restProps.label)}
          </div>

          <JsonView
            src={taskExecutorHeartbeatItem}
            displaySize={30}
            className="w-full max-h-[300px] break-words overflow-auto"
          />
        </div>
      </div>
    );
  }

  return null;
};

const TaskBarChat = ({ data }: IProps) => {
  const { t } = useTranslation();
  const { deleteTaskExecutor, loading } = useDeleteTaskExecutor();

  const handleDelete = async (executorId: string) => {
    await deleteTaskExecutor(executorId);
  };

  return Object.entries(data).map(([key, val]) => {
    const data = val.map((x) => ({
      ...x,
      now: dayjs(x.now).valueOf(),
    }));
    const firstItem = data[0];
    const lastItem = data[data.length - 1];

    const domain = [firstItem?.now, lastItem?.now];
    return (
      <Flex key={key} className={styles.taskBar} vertical>
        <div className="flex gap-8 items-center">
          <b className={styles.taskBarTitle}>ID: {key}</b>
          <b className={styles.taskBarTitle}>Lag: {lastItem?.lag}</b>
          <b className={styles.taskBarTitle}>Pending: {lastItem?.pending}</b>
          <div className="flex-grow"></div>
          <Popconfirm
            title={t('common.deleteConfirm')}
            onConfirm={() => handleDelete(key)}
            okText={t('common.yes')}
            cancelText={t('common.no')}
          >
            <Tooltip title={t('common.delete')}>
              <Button
                type="text"
                danger
                icon={<DeleteOutlined />}
                loading={loading}
              />
            </Tooltip>
          </Popconfirm>
        </div>
        <ResponsiveContainer>
          <BarChart data={data}>
            <XAxis
              dataKey="now"
              type="number"
              scale={'time'}
              domain={domain}
              tickFormatter={(x) => formatTime(x)}
              allowDataOverflow
              angle={60}
              padding={{ left: 20, right: 20 }}
              tickMargin={20}
            />
            <CartesianGrid strokeDasharray="3 3" />
            <ChartTooltip
              wrapperStyle={{ pointerEvents: 'auto' }}
              content={<CustomTooltip></CustomTooltip>}
              trigger="click"
            />
            <Legend wrapperStyle={{ bottom: -22 }} />
            <Bar
              dataKey="done"
              fill="#2fe235"
              activeBar={<Rectangle fill="pink" stroke="blue" />}
            />
            <Bar
              dataKey="failed"
              fill="#ef3b74"
              activeBar={<Rectangle fill="gold" stroke="purple" />}
            />
          </BarChart>
        </ResponsiveContainer>
        <Divider></Divider>
      </Flex>
    );
  });
};

export default TaskBarChat;
