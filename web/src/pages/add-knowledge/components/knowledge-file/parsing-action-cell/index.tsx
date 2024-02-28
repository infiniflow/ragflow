import showDeleteConfirm from '@/components/deleting-confirm';
import { IKnowledgeFile } from '@/interfaces/database/knowledge';
import { DeleteOutlined, EditOutlined, ToolOutlined } from '@ant-design/icons';
import { Button, Dropdown, MenuProps, Space, Tooltip } from 'antd';
import { useDispatch } from 'umi';

interface IProps {
  knowledgeBaseId: string;
  record: IKnowledgeFile;
  setDocumentAndParserId: () => void;
}

const ParsingActionCell = ({
  knowledgeBaseId,
  record,
  setDocumentAndParserId,
}: IProps) => {
  const dispatch = useDispatch();
  const documentId = record.id;

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
    showDeleteConfirm({ onOk: removeDocument });
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

  const showRenameModal = () => {
    setCurrentRecord();
    dispatch({
      type: 'kFModel/setIsShowRenameModal',
      payload: true,
    });
  };

  const chunkItems: MenuProps['items'] = [
    {
      key: '1',
      label: (
        <div>
          <Button type="link" onClick={showSegmentSetModal}>
            Parser type
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Space size={'middle'}>
      <Dropdown menu={{ items: chunkItems }} trigger={['click']}>
        <ToolOutlined size={20} onClick={setDocumentAndParserId} />
      </Dropdown>
      <Tooltip title="Rename">
        <EditOutlined size={20} onClick={showRenameModal} />
      </Tooltip>
      <DeleteOutlined size={20} onClick={onRmDocument} />
    </Space>
  );
};

export default ParsingActionCell;
