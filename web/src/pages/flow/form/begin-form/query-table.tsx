import { DeleteOutlined, EditOutlined } from '@ant-design/icons';
import type { TableProps } from 'antd';
import { Space, Table, Tooltip } from 'antd';
import { BeginQuery } from '../../interface';

interface IProps {
  data: BeginQuery[];
  deleteRecord(index: number): void;
  showModal(index: number, record: BeginQuery): void;
}

const QueryTable = ({ data, deleteRecord, showModal }: IProps) => {
  const columns: TableProps<BeginQuery>['columns'] = [
    {
      title: 'Key',
      dataIndex: 'key',
      key: 'key',
      ellipsis: {
        showTitle: false,
      },
      render: (key) => (
        <Tooltip placement="topLeft" title={key}>
          {key}
        </Tooltip>
      ),
    },
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      ellipsis: {
        showTitle: false,
      },
      render: (name) => (
        <Tooltip placement="topLeft" title={name}>
          {name}
        </Tooltip>
      ),
    },
    {
      title: 'Type',
      dataIndex: 'type',
      key: 'type',
    },
    {
      title: 'Optional',
      dataIndex: 'optional',
      key: 'optional',
      render: (optional) => (optional ? 'Yes' : 'No'),
    },
    {
      title: 'Action',
      key: 'action',
      render: (_, record, idx) => (
        <Space>
          <EditOutlined onClick={() => showModal(idx, record)} />
          <DeleteOutlined
            className="cursor-pointer"
            onClick={() => deleteRecord(idx)}
          />
        </Space>
      ),
    },
  ];

  return (
    <Table<BeginQuery> columns={columns} dataSource={data} pagination={false} />
  );
};

export default QueryTable;
