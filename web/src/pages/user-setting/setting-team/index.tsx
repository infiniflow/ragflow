import {
  useFetchUserInfo,
  useListTenantUser,
} from '@/hooks/user-setting-hooks';
import { Button, Card, Flex, Space } from 'antd';
import { useTranslation } from 'react-i18next';

import { TeamOutlined, UserAddOutlined, UserOutlined } from '@ant-design/icons';
import AddingUserModal from './add-user-modal';
import { useAddUser } from './hooks';
import styles from './index.less';
import TenantTable from './tenant-table';
import UserTable from './user-table';

const iconStyle = { fontSize: 20, color: '#1677ff' };

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
            {t('setting.invite')}
          </Button>
        </Flex>
      </Card>
      <Card
        title={
          <Space>
            <UserOutlined style={iconStyle} /> {t('setting.teamMembers')}
          </Space>
        }
        bordered={false}
      >
        <UserTable></UserTable>
      </Card>
      <Card
        title={
          <Space>
            <TeamOutlined style={iconStyle} /> {t('setting.joinedTeams')}
          </Space>
        }
        bordered={false}
      >
        <TenantTable></TenantTable>
      </Card>
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
