import { useTranslate } from '@/hooks/commonHooks';
import { IFile } from '@/interfaces/database/file-manager';
import { api_host } from '@/utils/api';
import { downloadFile } from '@/utils/fileUtil';
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Button, Space, Tooltip } from 'antd';
import { useHandleDeleteFile } from '../hooks';

import styles from './index.less';

interface IProps {
  record: IFile;
  setCurrentRecord: (record: any) => void;
  showRenameModal: (record: IFile) => void;
}

const ActionCell = ({ record, setCurrentRecord, showRenameModal }: IProps) => {
  const documentId = record.id;
  const beingUsed = false;
  const { t } = useTranslate('knowledgeDetails');
  const { handleRemoveFile } = useHandleDeleteFile([documentId]);

  const onDownloadDocument = () => {
    downloadFile({
      url: `${api_host}/document/get/${documentId}`,
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

  return (
    <Space size={0}>
      <Button type="text" className={styles.iconButton}>
        <ToolOutlined size={20} />
      </Button>

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
      <Button
        type="text"
        disabled={beingUsed}
        onClick={handleRemoveFile}
        className={styles.iconButton}
      >
        <DeleteOutlined size={20} />
      </Button>
      <Button
        type="text"
        disabled={beingUsed}
        onClick={onDownloadDocument}
        className={styles.iconButton}
      >
        <DownloadOutlined size={20} />
      </Button>
    </Space>
  );
};

export default ActionCell;
