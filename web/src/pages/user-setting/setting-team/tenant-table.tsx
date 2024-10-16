import { useListTenant } from '@/hooks/user-setting-hooks';
import { ITenant } from '@/interfaces/database/user-setting';
import { formatDate } from '@/utils/date';
import type { TableProps } from 'antd';
import { Button, Table } from 'antd';
import { useTranslation } from 'react-i18next';

const TenantTable = () => {
  const { t } = useTranslation();
  const { data } = useListTenant();

  const columns: TableProps<ITenant>['columns'] = [
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
      render: (_, record) => <Button type="primary">Invite</Button>,
    },
  ];

  return (
    <Table<ITenant> columns={columns} dataSource={data} rowKey={'tenant_id'} />
  );
};

export default TenantTable;
