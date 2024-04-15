import { IModalProps } from '@/interfaces/common';
import { IToken } from '@/interfaces/database/chat';
import { CopyOutlined, DeleteOutlined } from '@ant-design/icons';
import type { TableProps } from 'antd';
import { Button, Modal, Space, Table } from 'antd';
import { useOperateApiKey } from '../hooks';

const ChatApiKeyModal = ({
  visible,
  dialogId,
  hideModal,
}: IModalProps<any> & { dialogId: string }) => {
  const { createToken, removeToken, tokenList } = useOperateApiKey(
    visible,
    dialogId,
  );

  const columns: TableProps<IToken>['columns'] = [
    {
      title: 'Token',
      dataIndex: 'token',
      key: 'token',
      render: (text) => <a>{text}</a>,
    },
    {
      title: 'Created',
      dataIndex: 'create_date',
      key: 'create_date',
    },
    {
      title: 'Action',
      key: 'action',
      render: (_, record) => (
        <Space size="middle">
          <CopyOutlined onClick={() => {}} />
          <DeleteOutlined onClick={() => removeToken(record.token)} />
        </Space>
      ),
    },
  ];

  return (
    <>
      <Modal
        title="API key"
        open={visible}
        onCancel={hideModal}
        style={{ top: 300 }}
        width={'50vw'}
      >
        <Table columns={columns} dataSource={tokenList} key={'token'} />
        <Button onClick={createToken}>Create new key</Button>
      </Modal>
    </>
  );
};

export default ChatApiKeyModal;
