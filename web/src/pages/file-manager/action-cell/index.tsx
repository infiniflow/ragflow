import { useShowDeleteConfirm, useTranslate } from '@/hooks/commonHooks';
import { useRemoveDocument } from '@/hooks/documentHooks';
import { api_host } from '@/utils/api';
import { downloadFile } from '@/utils/fileUtil';
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Button, Space, Tooltip } from 'antd';

import styles from './index.less';

interface IProps {
  record: any;
  setCurrentRecord: (record: any) => void;
  showRenameModal: () => void;
}

const ActionCell = ({ record, setCurrentRecord, showRenameModal }: IProps) => {
  const documentId = record.id;
  const beingUsed = false;
  const { t } = useTranslate('knowledgeDetails');
  const removeDocument = useRemoveDocument();
  const showDeleteConfirm = useShowDeleteConfirm();

  const onRmDocument = () => {
    if (!beingUsed) {
      showDeleteConfirm({ onOk: () => removeDocument(documentId) });
    }
  };

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
    showRenameModal();
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
        onClick={onRmDocument}
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
