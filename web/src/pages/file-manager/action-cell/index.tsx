import { useTranslate } from '@/hooks/commonHooks';
import { IFile } from '@/interfaces/database/file-manager';
import { api_host } from '@/utils/api';
import { downloadFile } from '@/utils/fileUtil';
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  EyeOutlined,
  LinkOutlined,
} from '@ant-design/icons';
import { Button, Space, Tooltip } from 'antd';
import { useHandleDeleteFile } from '../hooks';

import NewDocumentLink from '@/components/new-document-link';
import { SupportedPreviewDocumentTypes } from '@/constants/common';
import { getExtension } from '@/utils/documentUtils';
import styles from './index.less';

const isSupportedPreviewDocumentType = (fileExtension: string) => {
  return SupportedPreviewDocumentTypes.includes(fileExtension);
};

interface IProps {
  record: IFile;
  setCurrentRecord: (record: any) => void;
  showRenameModal: (record: IFile) => void;
  showConnectToKnowledgeModal: (record: IFile) => void;
  setSelectedRowKeys(keys: string[]): void;
}

const ActionCell = ({
  record,
  setCurrentRecord,
  showRenameModal,
  showConnectToKnowledgeModal,
  setSelectedRowKeys,
}: IProps) => {
  const documentId = record.id;
  const beingUsed = false;
  const { t } = useTranslate('fileManager');
  const { handleRemoveFile } = useHandleDeleteFile(
    [documentId],
    setSelectedRowKeys,
  );
  const extension = getExtension(record.name);
  const isKnowledgeBase = record.source_type === 'knowledgebase';

  const onDownloadDocument = () => {
    downloadFile({
      url: `${api_host}/file/get/${documentId}`,
      filename: record.name,
    });
  };

  const setRecord = () => {
    setCurrentRecord(record);
  };

  const onShowRenameModal = () => {
    setRecord();
    showRenameModal(record);
  };

  const onShowConnectToKnowledgeModal = () => {
    showConnectToKnowledgeModal(record);
  };

  return (
    <Space size={0}>
      {isKnowledgeBase || (
        <Tooltip title={t('addToKnowledge')}>
          <Button
            type="text"
            className={styles.iconButton}
            onClick={onShowConnectToKnowledgeModal}
          >
            <LinkOutlined size={20} />
          </Button>
        </Tooltip>
      )}

      {isKnowledgeBase || (
        <Tooltip title={t('rename', { keyPrefix: 'common' })}>
          <Button
            type="text"
            disabled={beingUsed}
            onClick={onShowRenameModal}
            className={styles.iconButton}
          >
            <EditOutlined size={20} />
          </Button>
        </Tooltip>
      )}
      {isKnowledgeBase || (
        <Tooltip title={t('delete', { keyPrefix: 'common' })}>
          <Button
            type="text"
            disabled={beingUsed}
            onClick={handleRemoveFile}
            className={styles.iconButton}
          >
            <DeleteOutlined size={20} />
          </Button>
        </Tooltip>
      )}
      {record.type !== 'folder' && (
        <Tooltip title={t('download', { keyPrefix: 'common' })}>
          <Button
            type="text"
            disabled={beingUsed}
            onClick={onDownloadDocument}
            className={styles.iconButton}
          >
            <DownloadOutlined size={20} />
          </Button>
        </Tooltip>
      )}
      {isSupportedPreviewDocumentType(extension) && (
        <NewDocumentLink
          color="black"
          link={`/document/${documentId}?ext=${extension}`}
        >
          <Tooltip title={t('preview')}>
            <Button type="text" className={styles.iconButton}>
              <EyeOutlined size={20} />
            </Button>
          </Tooltip>
        </NewDocumentLink>
      )}
    </Space>
  );
};

export default ActionCell;
