import {
  useFetchUserInfo,
  useListTenantUser,
} from '@/hooks/user-setting-hooks';
import { Button, Card, Flex } from 'antd';
import { useTranslation } from 'react-i18next';

import { UserAddOutlined } from '@ant-design/icons';
import AddingUserModal from './add-user-modal';
import { useAddUser } from './hooks';
import styles from './index.less';
import TenantTable from './tenant-table';
import UserTable from './user-table';

const UserSettingTeam = () => {
  const { data: userInfo } = useFetchUserInfo();
  const { t } = useTranslation();
  useListTenantUser();
  const {
    addingTenantModalVisible,
    hideAddingTenantModal,
    showAddingTenantModal,
    handleAddUserOk,
  } = useAddUser();

  return (
    <div className={styles.teamWrapper}>
      <Card className={styles.teamCard}>
        <Flex align="center" justify={'space-between'}>
          <span>
            {userInfo.nickname} {t('setting.workspace')}
          </span>
          <Button type="primary" onClick={showAddingTenantModal}>
            <UserAddOutlined />
            {t('setting.add')}
          </Button>
        </Flex>
      </Card>
      <UserTable></UserTable>
      <TenantTable></TenantTable>
      {addingTenantModalVisible && (
        <AddingUserModal
          visible
          hideModal={hideAddingTenantModal}
          onOk={handleAddUserOk}
        ></AddingUserModal>
      )}
    </div>
  );
};

export default UserSettingTeam;
