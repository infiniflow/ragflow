import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { Modal, Table } from 'antd';
import { ColumnsType } from 'antd/es/table';
import { useTranslation } from 'react-i18next';
import { useDispatch, useSelector } from 'umi';
import styles from './index.less';

interface DataType {
  key: React.Key;
  name: string;
  role: string;
  time: string;
}

const TntModal = () => {
  const dispatch = useDispatch();
  const settingModel = useSelector((state: any) => state.settingModel);
  const { isShowTntModal, tenantIfo, factoriesList } = settingModel;
  const { t } = useTranslation();
  const loading = useOneNamespaceEffectsLoading('settingModel', [
    'getTenantInfo',
  ]);

  const columns: ColumnsType<DataType> = [
    { title: '姓名', dataIndex: 'name', key: 'name' },
    { title: '活动时间', dataIndex: 'update_date', key: 'update_date' },
    { title: '角色', dataIndex: 'role', key: 'age' },
  ];

  const handleCancel = () => {
    dispatch({
      type: 'settingModel/updateState',
      payload: {
        isShowTntModal: false,
      },
    });
  };

  const handleOk = async () => {
    dispatch({
      type: 'settingModel/updateState',
      payload: {
        isShowTntModal: false,
      },
    });
  };

  return (
    <Modal
      title="用户"
      open={isShowTntModal}
      onOk={handleOk}
      onCancel={handleCancel}
    >
      <div className={styles.tenantIfo}>{tenantIfo.name}</div>
      <Table
        rowKey="name"
        loading={loading}
        columns={columns}
        dataSource={factoriesList}
      />
    </Modal>
  );
};
export default TntModal;
