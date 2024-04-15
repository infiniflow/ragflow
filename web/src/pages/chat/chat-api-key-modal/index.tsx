import { IModalProps } from '@/interfaces/common';
import { CopyOutlined, DeleteOutlined } from '@ant-design/icons';
import type { TableProps } from 'antd';
import { Button, Modal, Space, Table } from 'antd';

interface DataType {
  key: string;
  name: string;
  age: number;
  address: string;
}

const ChatApiKeyModal = ({ visible, hideModal }: IModalProps<any>) => {
  const columns: TableProps<DataType>['columns'] = [
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
      render: () => (
        <Space size="middle">
          <CopyOutlined />
          <DeleteOutlined />
        </Space>
      ),
    },
  ];

  const data: DataType[] = [
    {
      key: '1',
      name: 'John Brown',
      age: 32,
      address: 'New York No. 1 Lake Park',
    },
    {
      key: '2',
      name: 'Jim Green',
      age: 42,
      address: 'London No. 1 Lake Park',
    },
    {
      key: '3',
      name: 'Joe Black',
      age: 32,
      address: 'Sydney No. 1 Lake Park',
    },
  ];

  const handleCancel = () => {
    hideModal();
  };

  return (
    <>
      <Modal
        title="API key"
        open={visible}
        onCancel={handleCancel}
        style={{ top: 300 }}
        width={'50vw'}
      >
        <Table columns={columns} dataSource={data} />
        <Button onClick={() => {}}>Create new key</Button>
      </Modal>
    </>
  );
};

export default ChatApiKeyModal;
