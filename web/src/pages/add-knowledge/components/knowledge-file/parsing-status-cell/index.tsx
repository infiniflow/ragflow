import { ReactComponent as CancelIcon } from '@/assets/svg/cancel.svg';
import { ReactComponent as RefreshIcon } from '@/assets/svg/refresh.svg';
import { ReactComponent as RunIcon } from '@/assets/svg/run.svg';
import { useTranslate } from '@/hooks/common-hooks';
import { IDocumentInfo } from '@/interfaces/database/document';
import {
  Badge,
  DescriptionsProps,
  Flex,
  Popconfirm,
  Popover,
  Space,
  Tag,
} from 'antd';
import classNames from 'classnames';
import { useTranslation } from 'react-i18next';
import reactStringReplace from 'react-string-replace';
import { DocumentType, RunningStatus, RunningStatusMap } from '../constant';
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
  record: IDocumentInfo;
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
      key: 'process_duration',
      label: t('processDuration'),
      children: `${record.process_duration.toFixed(2)} s`,
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
  const { handleRunDocumentByIds } = useHandleRunDocumentByIds(record.id);

  const isRunning = isParserRunning(text);

  const OperationIcon = iconMap[text];

  const label = t(`knowledgeDetails.runningStatus${text}`);

  const handleOperationIconClick =
    (shouldDelete: boolean = false) =>
    () => {
      handleRunDocumentByIds(record.id, isRunning, shouldDelete);
    };

  return record.type === DocumentType.Virtual ? null : (
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
      <Popconfirm
        title={t(`knowledgeDetails.redo`, { chunkNum: record.chunk_num })}
        onConfirm={handleOperationIconClick(true)}
        onCancel={handleOperationIconClick(false)}
        disabled={record.chunk_num === 0}
        okText={t('common.ok')}
        cancelText={t('common.cancel')}
      >
        <div
          className={classNames(styles.operationIcon)}
          onClick={
            record.chunk_num === 0 ? handleOperationIconClick(false) : () => {}
          }
        >
          <OperationIcon />
        </div>
      </Popconfirm>
    </Flex>
  );
};

export default ParsingStatusCell;
