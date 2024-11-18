import { TaskExecutorHeartbeatItem } from '@/interfaces/database/user-setting';
import { Divider, Flex } from 'antd';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Rectangle,
  ResponsiveContainer,
  Tooltip,
  XAxis,
} from 'recharts';

import { formatDate, formatTime } from '@/utils/date';
import dayjs from 'dayjs';
import { get } from 'lodash';
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
      <div className="bg-slate-50 p-2 rounded-md border border-indigo-100">
        <div className="font-semibold text-lg">
          {formatDate(restProps.label)}
        </div>
        {Object.entries(taskExecutorHeartbeatItem).map(([key, val], index) => {
          return (
            <div key={index} className="flex gap-1">
              <span className="font-semibold">{`${key}: `}</span>
              <span>
                {key === 'now' || key === 'boot_at' ? formatDate(val) : val}
              </span>
            </div>
          );
        })}
      </div>
    );
  }

  return null;
};

const TaskBarChat = ({ data }: IProps) => {
  return Object.entries(data).map(([key, val]) => {
    const data = val.map((x) => ({
      ...x,
      now: dayjs(x.now).valueOf(),
      failed: 5,
    }));
    const firstItem = data[0];
    const lastItem = data[data.length - 1];

    const domain = [firstItem.now, lastItem.now];

    return (
      <Flex key={key} className={styles.taskBar} vertical>
        <div className="flex gap-8">
          <b className={styles.taskBarTitle}>ID: {key}</b>
          <b className={styles.taskBarTitle}>Lag: {lastItem.lag}</b>
          <b className={styles.taskBarTitle}>Pending: {lastItem.pending}</b>
        </div>
        <ResponsiveContainer>
          <BarChart data={data} barSize={20}>
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
            <Tooltip content={<CustomTooltip></CustomTooltip>} />
            <Legend wrapperStyle={{ bottom: -22 }} />
            <Bar
              dataKey="done"
              fill="#8884d8"
              activeBar={<Rectangle fill="pink" stroke="blue" />}
            />
            <Bar
              dataKey="failed"
              fill="#82ca9d"
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
