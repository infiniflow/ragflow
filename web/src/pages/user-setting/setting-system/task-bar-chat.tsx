import { TaskExecutorElapsed } from '@/interfaces/database/user-setting';
import { Divider, Flex } from 'antd';
import { max } from 'lodash';
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
} from 'recharts';

import styles from './index.less';

interface IProps {
  data: TaskExecutorElapsed;
}

const getColor = (value: number) => {
  if (value > 120) {
    return 'red';
  } else if (value <= 120 && value > 50) {
    return '#faad14';
  }
  return '#52c41a';
};

const getMaxLength = (data: TaskExecutorElapsed) => {
  const lengths = Object.keys(data).reduce<number[]>((pre, cur) => {
    pre.push(data[cur].length);
    return pre;
  }, []);
  return max(lengths) ?? 0;
};

const fillEmptyElementByMaxLength = (list: any[], maxLength: number) => {
  if (list.length === maxLength) {
    return list;
  }
  return list.concat(
    new Array(maxLength - list.length).fill({
      value: 0,
      actualValue: 0,
      fill: getColor(0),
    }),
  );
};

const CustomTooltip = ({ active, payload }: any) => {
  if (active && payload && payload.length) {
    return (
      <div className="custom-tooltip">
        <p
          className={styles.taskBarTooltip}
        >{`${payload[0].payload.actualValue}`}</p>
      </div>
    );
  }

  return null;
};

const TaskBarChat = ({ data }: IProps) => {
  const maxLength = getMaxLength(data);
  return (
    <Flex gap="middle" vertical>
      {Object.keys(data).map((key) => {
        const list = data[key].map((x) => ({
          value: x > 120 ? 120 : x,
          actualValue: x,
          fill: getColor(x),
        }));
        const nextList = fillEmptyElementByMaxLength(list, maxLength);
        return (
          <Flex key={key} className={styles.taskBar} vertical>
            <b className={styles.taskBarTitle}>ID: {key}</b>
            <ResponsiveContainer>
              <BarChart data={nextList} barSize={20}>
                <CartesianGrid strokeDasharray="3 3" />
                <Tooltip content={<CustomTooltip></CustomTooltip>} />
                <Bar dataKey="value" />
              </BarChart>
            </ResponsiveContainer>
            <Divider></Divider>
          </Flex>
        );
      })}
    </Flex>
  );
};

export default TaskBarChat;
