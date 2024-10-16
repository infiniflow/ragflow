import { useListTenantUser } from '@/hooks/user-setting-hooks';
import { ITenantUser } from '@/interfaces/database/user-setting';
import { formatDate } from '@/utils/date';
import { DeleteOutlined } from '@ant-design/icons';
import type { TableProps } from 'antd';
import { Button, Table } from 'antd';
import { useTranslation } from 'react-i18next';
import { useHandleDeleteUser } from './hooks';

const UserTable = () => {
  const { data, loading } = useListTenantUser();
  const { handleDeleteTenantUser } = useHandleDeleteUser();
  const { t } = useTranslation();

  const columns: TableProps<ITenantUser>['columns'] = [
    {
      title: t('common.name'),
      dataIndex: 'nickname',
      key: 'nickname',
    },
    {
      title: t('setting.email'),
      dataIndex: 'email',
      key: 'email',
    },
    {
      title: t('setting.role'),
      dataIndex: 'role',
      key: 'role',
    },
    {
      title: t('setting.updateDate'),
      dataIndex: 'update_date',
      key: 'update_date',
      render(value) {
        return formatDate(value);
      },
    },
    {
      title: t('common.action'),
      key: 'action',
      render: (_, record) => (
        <Button type="text" onClick={handleDeleteTenantUser(record.user_id)}>
          <DeleteOutlined size={20} />
        </Button>
      ),
    },
  ];

  return (
    <Table<ITenantUser>
      rowKey={'user_id'}
      columns={columns}
      dataSource={data}
      loading={loading}
    />
  );
};

export default UserTable;
