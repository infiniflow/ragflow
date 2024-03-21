import showDeleteConfirm from '@/components/deleting-confirm';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import {
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Button, Dropdown, MenuProps, Space, Tooltip } from 'antd';
import { useDispatch } from 'umi';
import { isParserRunning } from '../utils';

import { api_host } from '@/utils/api';
import { downloadFile } from '@/utils/fileUtil';
import styles from './index.less';

interface IProps {
  knowledgeBaseId: string;
  record: IKnowledgeFile;
  setDocumentAndParserId: () => void;
  showRenameModal: () => void;
}

const ParsingActionCell = ({
  knowledgeBaseId,
  record,
  setDocumentAndParserId,
  showRenameModal,
}: IProps) => {
  const dispatch = useDispatch();
  const documentId = record.id;
  const isRunning = isParserRunning(record.run);

  const removeDocument = () => {
    dispatch({
      type: 'kFModel/document_rm',
      payload: {
        doc_id: documentId,
        kb_id: knowledgeBaseId,
      },
    });
  };

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

  const setCurrentRecord = () => {
    dispatch({
      type: 'kFModel/setCurrentRecord',
      payload: record,
    });
  };

  const showSegmentSetModal = () => {
    dispatch({
      type: 'kFModel/updateState',
      payload: {
        isShowSegmentSetModal: true,
      },
    });
  };

  const chunkItems: MenuProps['items'] = [
    {
      key: '1',
      label: (
        <div>
          <Button type="link" onClick={showSegmentSetModal}>
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
        <Button
          type="text"
          onClick={setDocumentAndParserId}
          className={styles.iconButton}
        >
          <ToolOutlined size={20} />
        </Button>
      </Dropdown>
      <Tooltip title="Rename">
        <Button
          type="text"
          disabled={isRunning}
          onClick={showRenameModal}
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
