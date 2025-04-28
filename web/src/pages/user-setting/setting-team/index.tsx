import {
  useFetchUserInfo,
  useListTenantUser,
} from '@/hooks/user-setting-hooks';
import { Button, Card, Flex, Space, Tabs } from 'antd';
import { useTranslation } from 'react-i18next';

import { TeamOutlined, UserAddOutlined, UserOutlined, PartitionOutlined, PlusOutlined } from '@ant-design/icons';
import AddingUserModal from './add-user-modal';
import { useAddUser } from './hooks';
import styles from './index.less';
import TenantTable from './tenant-table';
import UserTable from './user-table';
import DepartmentTable from './department-table';
import GroupTable from './group-table';
import { useState } from 'react';

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
  const [activeTab, setActiveTab] = useState('members');

  const tabItems = [
    {
      key: 'members',
      label: t('setting.teamMembers'),
      children: (
        <>
          <UserTable />
        </>
      ),
    },
    {
      key: 'departments',
      label: '部门',
      children: (
        <>
          <DepartmentTable />
        </>
      ),
    },
    {
      key: 'groups',
      label: '群组',
      children: <GroupTable />,
    },
  ];

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
      <Card bordered={false}>
        <Tabs 
          activeKey={activeTab}
          onChange={setActiveTab}
          items={tabItems}
        />
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
