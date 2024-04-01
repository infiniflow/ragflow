import showDeleteConfirm from '@/components/deleting-confirm';
import { useRemoveDocument } from '@/hooks/documentHooks';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { api_host } from '@/utils/api';
import { downloadFile } from '@/utils/fileUtil';
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Button, Dropdown, MenuProps, Space, Tooltip } from 'antd';
import { isParserRunning } from '../utils';

import styles from './index.less';

interface IProps {
  record: IKnowledgeFile;
  setCurrentRecord: (record: IKnowledgeFile) => void;
  showRenameModal: () => void;
  showChangeParserModal: () => void;
}

const ParsingActionCell = ({
  record,
  setCurrentRecord,
  showRenameModal,
  showChangeParserModal,
}: IProps) => {
  const documentId = record.id;
  const isRunning = isParserRunning(record.run);

  const removeDocument = useRemoveDocument(documentId);

  const onRmDocument = () => {
    if (!isRunning) {
      showDeleteConfirm({ onOk: removeDocument });
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
  const onShowChangeParserModal = () => {
    setRecord();
    showChangeParserModal();
  };

  const chunkItems: MenuProps['items'] = [
    {
      key: '1',
      label: (
        <div>
          <Button type="link" onClick={onShowChangeParserModal}>
            Chunk Method
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Space size={0}>
      <Dropdown
        menu={{ items: chunkItems }}
        trigger={['click']}
        disabled={isRunning}
      >
        <Button type="text" className={styles.iconButton}>
          <ToolOutlined size={20} />
        </Button>
      </Dropdown>
      <Tooltip title="Rename">
        <Button
          type="text"
          disabled={isRunning}
          onClick={onShowRenameModal}
          className={styles.iconButton}
        >
          <EditOutlined size={20} />
        </Button>
      </Tooltip>
      <Button
        type="text"
        disabled={isRunning}
        onClick={onRmDocument}
        className={styles.iconButton}
      >
        <DeleteOutlined size={20} />
      </Button>
      <Button
        type="text"
        disabled={isRunning}
        onClick={onDownloadDocument}
        className={styles.iconButton}
      >
        <DownloadOutlined size={20} />
      </Button>
    </Space>
  );
};

export default ParsingActionCell;
