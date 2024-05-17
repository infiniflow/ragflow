import { ReactComponent as CancelIcon } from '@/assets/svg/cancel.svg';
import { ReactComponent as RefreshIcon } from '@/assets/svg/refresh.svg';
import { ReactComponent as RunIcon } from '@/assets/svg/run.svg';
import { useTranslate } from '@/hooks/commonHooks';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { Badge, DescriptionsProps, Flex, Popover, Space, Tag } from 'antd';
import classNames from 'classnames';
import { useTranslation } from 'react-i18next';
import reactStringReplace from 'react-string-replace';
import { RunningStatus, RunningStatusMap } from '../constant';
import { useHandleRunDocumentByIds } from '../hooks';
import { isParserRunning } from '../utils';
import styles from './index.less';

const iconMap = {
  [RunningStatus.UNSTART]: RunIcon,
  [RunningStatus.RUNNING]: CancelIcon,
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
      children: `${record.process_duation.toFixed(2)} s`,
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
  const text = record.run;
  const runningStatus = RunningStatusMap[text];
  const { t } = useTranslation();
  const { handleRunDocumentByIds, loading } = useHandleRunDocumentByIds(
    record.id,
  );

  const isRunning = isParserRunning(text);

  const OperationIcon = iconMap[text];

  const label = t(`knowledgeDetails.runningStatus${text}`);

  const handleOperationIconClick = () => {
    handleRunDocumentByIds(record.id, record.kb_id, isRunning);
  };

  return (
    <Flex justify={'space-between'} align="center">
      <Popover content={<PopoverContent record={record}></PopoverContent>}>
        <Tag color={runningStatus.color}>
          {isRunning ? (
            <Space>
              <Badge color={runningStatus.color} />
              {label}
              <span>{(record.progress * 100).toFixed(2)}%</span>
            </Space>
          ) : (
            label
          )}
        </Tag>
      </Popover>
      <div
        onClick={handleOperationIconClick}
        className={classNames(styles.operationIcon, {
          [styles.operationIconSpin]: loading,
        })}
      >
        <OperationIcon />
      </div>
    </Flex>
  );
};

export default ParsingStatusCell;
