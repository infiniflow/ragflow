import NewDocumentLink from '@/components/new-document-link';
import SvgIcon from '@/components/svg-icon';
import { useTranslate } from '@/hooks/common-hooks';
import { IFile } from '@/interfaces/database/file-manager';
import { api_host } from '@/utils/api';
import {
  getExtension,
  isSupportedPreviewDocumentType,
} from '@/utils/document-util';
import { downloadFile } from '@/utils/file-util';
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  EyeOutlined,
  LinkOutlined,
} from '@ant-design/icons';
import { Button, Space, Tooltip } from 'antd';
import { useHandleDeleteFile } from '../hooks';

interface IProps {
  record: IFile;
  setCurrentRecord: (record: any) => void;
  showRenameModal: (record: IFile) => void;
  showMoveFileModal: (ids: string[]) => void;
  showConnectToKnowledgeModal: (record: IFile) => void;
  setSelectedRowKeys(keys: string[]): void;
}

const ActionCell = ({
  record,
  setCurrentRecord,
  showRenameModal,
  showConnectToKnowledgeModal,
  setSelectedRowKeys,
  showMoveFileModal,
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

  const onShowMoveFileModal = () => {
    showMoveFileModal([documentId]);
  };

  return (
    <Space size={0}>
      {isKnowledgeBase || (
        <Tooltip title={t('addToKnowledge')}>
          <Button type="text" onClick={onShowConnectToKnowledgeModal}>
            <LinkOutlined size={20} />
          </Button>
        </Tooltip>
      )}

      {isKnowledgeBase || (
        <Tooltip title={t('rename', { keyPrefix: 'common' })}>
          <Button type="text" disabled={beingUsed} onClick={onShowRenameModal}>
            <EditOutlined size={20} />
          </Button>
        </Tooltip>
      )}
      {isKnowledgeBase || (
        <Tooltip title={t('move', { keyPrefix: 'common' })}>
          <Button
            type="text"
            disabled={beingUsed}
            onClick={onShowMoveFileModal}
          >
            <SvgIcon name={`move`} width={16}></SvgIcon>
          </Button>
        </Tooltip>
      )}
      {isKnowledgeBase || (
        <Tooltip title={t('delete', { keyPrefix: 'common' })}>
          <Button type="text" disabled={beingUsed} onClick={handleRemoveFile}>
            <DeleteOutlined size={20} />
          </Button>
        </Tooltip>
      )}
      {record.type !== 'folder' && (
        <Tooltip title={t('download', { keyPrefix: 'common' })}>
          <Button type="text" disabled={beingUsed} onClick={onDownloadDocument}>
            <DownloadOutlined size={20} />
          </Button>
        </Tooltip>
      )}
      {isSupportedPreviewDocumentType(extension) && (
        <NewDocumentLink
          documentId={documentId}
          documentName={record.name}
          color="black"
        >
          <Tooltip title={t('preview')}>
            <Button type="text">
              <EyeOutlined size={20} />
            </Button>
          </Tooltip>
        </NewDocumentLink>
      )}
    </Space>
  );
};

export default ActionCell;
