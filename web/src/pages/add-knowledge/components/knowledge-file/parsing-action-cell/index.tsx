import { useShowDeleteConfirm, useTranslate } from '@/hooks/common-hooks';
import { useRemoveNextDocument } from '@/hooks/document-hooks';
import { IDocumentInfo } from '@/interfaces/database/document';
import { downloadDocument } from '@/utils/file-util';
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Button, Dropdown, MenuProps, Space, Tooltip } from 'antd';
import { isParserRunning } from '../utils';

import { useCallback } from 'react';
import { DocumentType } from '../constant';
import styles from './index.less';

interface IProps {
  record: IDocumentInfo;
  setCurrentRecord: (record: IDocumentInfo) => void;
  showRenameModal: () => void;
  showChangeParserModal: () => void;
  showSetMetaModal: () => void;
}

const ParsingActionCell = ({
  record,
  setCurrentRecord,
  showRenameModal,
  showChangeParserModal,
  showSetMetaModal,
}: IProps) => {
  const documentId = record.id;
  const isRunning = isParserRunning(record.run);
  const { t } = useTranslate('knowledgeDetails');
  const { removeDocument } = useRemoveNextDocument();
  const showDeleteConfirm = useShowDeleteConfirm();
  const isVirtualDocument = record.type === DocumentType.Virtual;

  const onRmDocument = () => {
    if (!isRunning) {
      showDeleteConfirm({
        onOk: () => removeDocument([documentId]),
        content: record?.parser_config?.graphrag?.use_graphrag
          ? t('deleteDocumentConfirmContent')
          : '',
      });
    }
  };

  const onDownloadDocument = () => {
    downloadDocument({
      id: documentId,
      filename: record.name,
    });
  };

  const setRecord = useCallback(() => {
    setCurrentRecord(record);
  }, [record, setCurrentRecord]);

  const onShowRenameModal = () => {
    setRecord();
    showRenameModal();
  };
  const onShowChangeParserModal = () => {
    setRecord();
    showChangeParserModal();
  };

  const onShowSetMetaModal = useCallback(() => {
    setRecord();
    showSetMetaModal();
  }, [setRecord, showSetMetaModal]);

  const chunkItems: MenuProps['items'] = [
    {
      key: '1',
      label: (
        <div className="flex flex-col">
          <Button type="link" onClick={onShowChangeParserModal}>
            {t('chunkMethod')}
          </Button>
        </div>
      ),
    },
    { type: 'divider' },
    {
      key: '2',
      label: (
        <div className="flex flex-col">
          <Button type="link" onClick={onShowSetMetaModal}>
            {t('setMetaData')}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Space size={0}>
      {isVirtualDocument || (
        <Dropdown
          menu={{ items: chunkItems }}
          trigger={['click']}
          disabled={isRunning || record.parser_id === 'tag'}
        >
          <Button type="text" className={styles.iconButton}>
            <ToolOutlined size={20} />
          </Button>
        </Dropdown>
      )}
      <Tooltip title={t('rename', { keyPrefix: 'common' })}>
        <Button
          type="text"
          disabled={isRunning}
          onClick={onShowRenameModal}
          className={styles.iconButton}
        >
          <EditOutlined size={20} />
        </Button>
      </Tooltip>
      <Tooltip title={t('delete', { keyPrefix: 'common' })}>
        <Button
          type="text"
          disabled={isRunning}
          onClick={onRmDocument}
          className={styles.iconButton}
        >
          <DeleteOutlined size={20} />
        </Button>
      </Tooltip>
      {isVirtualDocument || (
        <Tooltip title={t('download', { keyPrefix: 'common' })}>
          <Button
            type="text"
            disabled={isRunning}
            onClick={onDownloadDocument}
            className={styles.iconButton}
          >
            <DownloadOutlined size={20} />
          </Button>
        </Tooltip>
      )}
    </Space>
  );
};

export default ParsingActionCell;
