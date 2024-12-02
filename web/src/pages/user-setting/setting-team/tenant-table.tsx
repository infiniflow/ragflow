import { useFetchUserInfo, useListTenant } from '@/hooks/user-setting-hooks';
import { ITenant } from '@/interfaces/database/user-setting';
import { formatDate } from '@/utils/date';
import type { TableProps } from 'antd';
import { Button, Space, Table } from 'antd';
import { useTranslation } from 'react-i18next';
import { TenantRole } from '../constants';
import { useHandleAgreeTenant, useHandleQuitUser } from './hooks';

const TenantTable = () => {
  const { t } = useTranslation();
  const { data, loading } = useListTenant();
  const { handleAgree } = useHandleAgreeTenant();
  const { data: user } = useFetchUserInfo();
  const { handleQuitTenantUser } = useHandleQuitUser();

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
      render: (_, { role, tenant_id }) => {
        if (role === TenantRole.Invite) {
          return (
            <Space>
              <Button type="link" onClick={handleAgree(tenant_id, true)}>
                {t(`setting.agree`)}
              </Button>
              <Button type="link" onClick={handleAgree(tenant_id, false)}>
                {t(`setting.refuse`)}
              </Button>
            </Space>
          );
        } else if (role === TenantRole.Normal && user.id !== tenant_id) {
          return (
            <Button
              type="link"
              onClick={handleQuitTenantUser(user.id, tenant_id)}
            >
              {t('setting.quit')}
            </Button>
          );
        }
      },
    },
  ];

  return (
    <Table<ITenant>
      columns={columns}
      dataSource={data}
      rowKey={'tenant_id'}
      loading={loading}
      pagination={false}
    />
  );
};

export default TenantTable;
