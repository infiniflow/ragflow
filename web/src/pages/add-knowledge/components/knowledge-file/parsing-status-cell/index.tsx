import { ReactComponent as RefreshIcon } from '@/assets/svg/refresh.svg';
import { ReactComponent as RunIcon } from '@/assets/svg/run.svg';
import { useTranslate } from '@/hooks/commonHooks';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { CloseCircleOutlined } from '@ant-design/icons';
import { Badge, DescriptionsProps, Flex, Popover, Space, Tag } from 'antd';
import reactStringReplace from 'react-string-replace';
import { useDispatch } from 'umi';
import { RunningStatus, RunningStatusMap } from '../constant';
import { isParserRunning } from '../utils';
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
  const { t } = useTranslate('knowledgeDetails');

  const replaceText = (text: string) => {
    // Remove duplicate \n
    const nextText = text.replace(/(\n)\1+/g, '$1');

    const replacedText = reactStringReplace(
      nextText,
      /(\[ERROR\].+\s)/g,
      (match, i) => {
        return (
          <span key={i} className={styles.popoverContentErrorLabel}>
            {match}
          </span>
        );
      },
    );

    return replacedText;
  };

  const items: DescriptionsProps['items'] = [
    {
      key: 'process_begin_at',
      label: t('processBeginAt'),
      children: record.process_begin_at,
    },
    {
      key: 'process_duation',
      label: t('processDuration'),
      children: record.process_duation,
    },
    {
      key: 'progress_msg',
      label: t('progressMsg'),
      children: replaceText(record.progress_msg.trim()),
    },
  ];

  return (
    <Flex vertical className={styles.popoverContent}>
      {items.map((x, idx) => {
        return (
          <div key={x.key} className={idx < 2 ? styles.popoverContentItem : ''}>
            <b>{x.label}:</b>
            <div className={styles.popoverContentText}>{x.children}</div>
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

  const isRunning = isParserRunning(text);

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
