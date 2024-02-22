import { ReactComponent as RefreshIcon } from '@/assets/svg/refresh.svg';
import { ReactComponent as RunIcon } from '@/assets/svg/run.svg';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { Badge, DescriptionsProps, Flex, Popover, Space, Tag } from 'antd';
import { RunningStatus, RunningStatusMap } from '../constant';

import { CloseCircleOutlined } from '@ant-design/icons';
import { useDispatch } from 'umi';
import styles from './index.less';

const iconMap = {
  [RunningStatus.UNSTART]: RunIcon,
  [RunningStatus.RUNNING]: CloseCircleOutlined,
  [RunningStatus.CANCEL]: RefreshIcon,
  [RunningStatus.DONE]: RefreshIcon,
  [RunningStatus.FAIL]: RefreshIcon,
};

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
          <div key={x.key}>
            <b>{x.label}:</b>
            <p>{x.children}</p>
          </div>
        );
      })}
    </Flex>
  );
};

export const ParsingStatusCell = ({ record }: IProps) => {
  const dispatch = useDispatch();
  const text = record.run;
  const runningStatus = RunningStatusMap[text];

  const isRunning = text === RunningStatus.RUNNING;

  const OperationIcon = iconMap[text];

  const handleOperationIconClick = () => {
    dispatch({
      type: 'kFModel/document_run',
      payload: {
        doc_ids: [record.id],
        run: isRunning ? 2 : 1,
        knowledgeBaseId: record.kb_id,
      },
    });
  };

  return (
    <Flex justify={'space-between'}>
      <Popover content={<PopoverContent record={record}></PopoverContent>}>
        <Tag color={runningStatus.color}>
          {isRunning ? (
            <Space>
              <Badge color={runningStatus.color} />
              {runningStatus.label}
              <span>{(record.progress * 100).toFixed(2)}%</span>
            </Space>
          ) : (
            runningStatus.label
          )}
        </Tag>
      </Popover>
      <div onClick={handleOperationIconClick} className={styles.operationIcon}>
        <OperationIcon />
      </div>
    </Flex>
  );
};

export default ParsingStatusCell;
