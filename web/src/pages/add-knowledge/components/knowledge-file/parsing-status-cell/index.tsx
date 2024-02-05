import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { Badge, DescriptionsProps, Flex, Popover, Space, Tag } from 'antd';
import { RunningStatus, RunningStatusMap } from '../constant';

import styles from './index.less';

interface IProps {
  record: IKnowledgeFile;
}

const PopoverContent = ({ record }: IProps) => {
  const items: DescriptionsProps['items'] = [
    {
      key: 'process_begin_at',
      label: 'Process Begin At',
      children: record.process_begin_at,
    },
    {
      key: 'process_duation',
      label: 'Process Duration',
      children: record.process_duation,
    },
    {
      key: 'progress_msg',
      label: 'Progress Msg',
      children: record.progress_msg,
    },
  ];

  return (
    <Flex vertical className={styles['popover-content']}>
      {items.map((x) => {
        return (
          <div>
            <b>{x.label}:</b>
            <p>{x.children}</p>
          </div>
        );
      })}
    </Flex>
  );
};

export const ParsingStatusCell = ({ record }: IProps) => {
  const text = record.run;
  const runningStatus = RunningStatusMap[text];

  const isRunning = text === RunningStatus.RUNNING;

  return (
    <Popover
      content={isRunning && <PopoverContent record={record}></PopoverContent>}
    >
      <Tag color={runningStatus.color}>
        {isRunning ? (
          <Space>
            <Badge color={runningStatus.color} />
            {runningStatus.label}
            <span>{record.progress * 100}%</span>
          </Space>
        ) : (
          runningStatus.label
        )}
      </Tag>
    </Popover>
  );
};

export default ParsingStatusCell;
