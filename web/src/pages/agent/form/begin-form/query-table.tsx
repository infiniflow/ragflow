import { DeleteOutlined, EditOutlined } from '@ant-design/icons';
import type { TableProps } from 'antd';
import { Collapse, Space, Table, Tooltip } from 'antd';
import { BeginQuery } from '../../interface';

import { useTranslation } from 'react-i18next';
import styles from './index.less';

interface IProps {
  data: BeginQuery[];
  deleteRecord(index: number): void;
  showModal(index: number, record: BeginQuery): void;
}

const QueryTable = ({ data, deleteRecord, showModal }: IProps) => {
  const { t } = useTranslation();

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
      title: t('flow.name'),
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
      title: t('flow.type'),
      dataIndex: 'type',
      key: 'type',
    },
    {
      title: t('flow.optional'),
      dataIndex: 'optional',
      key: 'optional',
      render: (optional) => (optional ? 'Yes' : 'No'),
    },
    {
      title: t('common.action'),
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
    <Collapse
      defaultActiveKey={['1']}
      className={styles.dynamicInputVariable}
      items={[
        {
          key: '1',
          label: <span className={styles.title}>{t('flow.input')}</span>,
          children: (
            <Table<BeginQuery>
              columns={columns}
              dataSource={data}
              pagination={false}
            />
          ),
        },
      ]}
    />
  );
};

export default QueryTable;
