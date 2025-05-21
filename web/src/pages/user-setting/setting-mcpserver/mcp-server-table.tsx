import { formatDate } from '@/utils/date';
import { DeleteOutlined, EditOutlined } from '@ant-design/icons';
import type { TableProps } from 'antd';
import { Button, Table, Tag } from 'antd';
import { useTranslation } from 'react-i18next';
import { useHandleDeleteMcpServer } from './hooks';
import { useFetchMcpServerList } from '@/hooks/mcp-server-setting-hooks';
import { IMcpServerInfo } from '@/interfaces/database/mcp-server';
import { camelCase } from 'lodash';

interface IProps {
  handleUpdateMcpServer: (serverId: string) => void;
}

const McpServerTable = ({
  handleUpdateMcpServer
}: IProps) => {
  const { data, loading } = useFetchMcpServerList();
  const { handleDeleteMcpServer } = useHandleDeleteMcpServer();
  const { t } = useTranslation();

  const columns: TableProps<IMcpServerInfo>['columns'] = [
    {
      title: t('common.name'),
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: t('setting.mcpServerDescription'),
      dataIndex: 'description',
      key: 'description',
      render(value) {
        if (value && value.length > 25) {
          return value.substring(0, 25) + '...';
        }
      },
    },
    {
      title: t('setting.mcpServerType'),
      dataIndex: 'server_type',
      key: 'server_type',
      render(value) {
        return t(`setting.mcpServerTypes.${camelCase(value)}`)
      },
    },
    {
      title: t('setting.mcpServerUrl'),
      dataIndex: 'url',
      key: 'url',
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
        <div>
          <Button type="text" onClick={() => handleUpdateMcpServer(record.id)}>
            <EditOutlined size={20} />
          </Button>
          <Button type="text" onClick={handleDeleteMcpServer(record.id)}>
            <DeleteOutlined size={20} />
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Table<IMcpServerInfo>
      rowKey={'id'}
      columns={columns}
      dataSource={data}
      loading={loading}
      pagination={false}
    />
  );
};

export default McpServerTable;
